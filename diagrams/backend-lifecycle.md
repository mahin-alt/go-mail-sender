# Backend Pipeline & Lifecycle

This document details the current end-to-end lifecycle and data pipeline of the backend (`go-mail-sender`), covering request ingestion, Excel parsing, domain verification, batch creation, database chunking, and worker delivery.

---

## 1. High-Level Architecture Flow

```mermaid
flowchart TD
    Client["Client (Browser / Postman)"] -->|"HTTP POST (Multipart .xlsx)"| API["API Layer (net/http Handler)"]
    
    subgraph IngestPipeline["1. Ingestion & Validation Pipeline"]
        API -->|"readUpload()"| LimitCheck["MaxBytesReader & Limit Check"]
        LimitCheck -->|"ParseExcelReader()"| Excelize["Excelize Parser (f.GetRows)"]
        Excelize -->|"Rows matrix"| Dedupe["Row Validation & Seen Map"]
        Dedupe -->|"Valid Recipient slice"| DomainCheck["Domain Verifier (DNS MX/Host)"]
    end

    subgraph BatchPipeline["2. Batch & Job Creation Pipeline"]
        DomainCheck -->|"Valid + Problem slices"| Render["Template Engine (Per-row Render)"]
        Render -->|"1M Job Structs in RAM"| DBTx["Postgres Single Transaction"]
        DBTx -->|"INSERT INTO batches"| BatchRow["1 Batch Record"]
        DBTx -->|"Chunked INSERT (500 rows/query)"| JobRows["2,000 Multi-Row INSERTs"]
        DBTx -->|"tx.Commit()"| Commit["Committed to DB"]
        Commit -->|"h.worker.Wake()"| WorkerSignal["Signal Worker Pool"]
    end

    subgraph DeliveryPipeline["3. Background Worker Delivery"]
        WorkerSignal -.-> WorkerPool["Worker Goroutines (N workers)"]
        WorkerPool -->|"ClaimNextJob (SKIP LOCKED)"| DBJob["Claimed Job (status=processing)"]
        WorkerPool -->|"Acquire connection"| SMTPPool["SMTP Pool (M conns)"]
        SMTPPool -->|"MAIL FROM / RCPT TO / DATA"| MailServer["Remote Mail Server"]
        WorkerPool -->|"MarkJobSent / Retry Backoff"| UpdateJob["Update email_jobs & batches"]
    end
```

---

## 2. Spreadsheet Parse Flow (`POST /api/v1/recipients/parse`)

This endpoint previews the spreadsheet without saving anything to the database.

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client (Frontend / Postman)
    participant Handler as handler.go (ParseRecipients)
    participant Parser as recipients.go (ParseExcelReader)
    participant Excelize as github.com/xuri/excelize
    participant Domain as domaincheck (verifyDomains)
    participant DNS as Net Resolver (LookupMX)

    Client->>Handler: POST /api/v1/recipients/parse (multipart file)
    Handler->>Handler: readUpload(): MaxBytesReader + check header (.xlsx)
    
    rect rgb(255, 240, 240)
    Note over Handler,Excelize: Memory Spikes: Loads entire sheet into Go memory
    Handler->>Parser: ParseExcelReader(file io.Reader)
    Parser->>Excelize: OpenReader(r) + f.GetRows(sheets[0])
    Excelize-->>Parser: [][]string (entire 1M row matrix in heap)
    end

    Parser->>Parser: Validate columns ("name", "email")
    loop For each row in rows[1:]
        Parser->>Parser: Check empty, format, and map[string]struct{} deduplication
    end
    Parser-->>Handler: valid []Recipient, problems []RowError

    rect rgb(240, 245, 255)
    Note over Handler,DNS: Domain verification (if DOMAIN_CHECK_ENABLED=true)
    Handler->>Domain: verifyDomains(ctx, valid)
    loop For each unique domain in valid recipients
        Domain->>DNS: LookupMX(domain), fallback LookupHost
        DNS-->>Domain: Result (Valid / Invalid / Unknown)
    end
    Domain-->>Handler: ok []Recipient, rejected []RowError, unverified []RowError
    end

    rect rgb(255, 235, 235)
    Note over Handler,Client: Massive Serialization: 78-100MB JSON payload
    Handler->>Handler: mergeProblems(problems, rejected)
    Handler->>Client: 200 OK with parseRecipientsResponse<br/>(Valid: 1M items, Problems: 170k items)
    end
```

### Memory Bottlenecks in Parse:
1. **`f.GetRows(sheets[0])`**: Parses uncompressed XML DOM tree into Go slices (`[][]string`), using ~233 MB - 400 MB RAM.
2. **`seen := make(map[string]struct{})`**: 1M string keys allocate ~60 MB in hash table buckets.
3. **`writeJSON(...)`**: Streams ~78 MB to 100+ MB of JSON containing all 1,000,000 recipient records.

---

## 3. Batch Ingestion Flow (`POST /api/v1/batches`)

This endpoint parses the spreadsheet, pre-renders emails, and persists the jobs into PostgreSQL.

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client (Frontend / Postman)
    participant Handler as handler.go (CreateBatch)
    participant Tmpl as template.go (Render)
    participant Store as store.go (CreateBatchWithJobs)
    participant DB as PostgreSQL Database
    participant Worker as worker.go (Pool)

    Client->>Handler: POST /api/v1/batches (file, subject, body)
    Handler->>Handler: readUpload() + validate templates
    Handler->>Handler: recipients.ParseExcelReader(file)
    Handler->>Handler: verifyDomains(valid)

    rect rgb(255, 240, 240)
    Note over Handler,Tmpl: Major Memory Hotspot: Pre-rendering 1M emails
    loop For every valid recipient (e.g. 800,000 rows)
        Handler->>Tmpl: Render(subjectTmpl, data) -> parses AST anew
        Handler->>Tmpl: Render(bodyTmpl, data) -> parses AST anew
        Handler->>Handler: jobs = append(jobs, store.Job{Subject, Body, ...})
    end
    loop For every problem row (e.g. 170,000 rows)
        Handler->>Handler: jobs = append(jobs, store.Job{Status: "invalid", ...})
    end
    Note over Handler: jobs slice in RAM: ~1,000,000 structs (~900MB - 1.5GB RAM)
    end

    rect rgb(240, 255, 240)
    Note over Handler,DB: Single Transaction Insertion (2,000 chunks)
    Handler->>Store: CreateBatchWithJobs(filename, subject, body, jobs)
    Store->>DB: BEGIN TRANSACTION
    Store->>DB: INSERT INTO batches (...) RETURNING id
    DB-->>Store: batch_id
    
    loop Every 500 rows (chunkSize = 500, ~2,000 iterations)
        Store->>DB: INSERT INTO email_jobs (...) VALUES ($1..$8), ... ($3993..$4000)
    end
    Store->>DB: COMMIT TRANSACTION
    end

    Store-->>Handler: batchID
    Handler->>Worker: h.worker.Wake() (triggers non-blocking channel signal)
    Handler-->>Client: 202 Accepted { batch_id, total_rows, valid_count, problems }
```

### Memory Bottlenecks in Batch Ingestion:
1. **Template Parse Multiplier**: `template.New("email").Parse(...)` is called 2,000,000 times (twice per recipient).
2. **In-Memory Rendered Bodies**: 1,000,000 rendered strings stored in RAM simultaneously (~500 MB - 1 GB).
3. **Giant Slice**: `jobs := make([]store.Job, 0, 1M)` keeps all 1M structs (~208 bytes each) in memory until SQL commit completes.
4. **Long Transaction**: 2,000 chunk queries with 4,000 parameters each executed within a single transaction `tx`.

---

## 4. Background Delivery Worker Lifecycle

Once a batch is committed, background workers process jobs asynchronously.

```mermaid
stateDiagram-v2
    [*] --> Idle: Worker starts (N goroutines)
    Idle --> Claiming: Wake signal OR 500ms ticker
    
    state Claiming {
        [*] --> ExecQuery
        ExecQuery: UPDATE email_jobs SET status='processing', attempts=attempts+1
        ExecQuery: WHERE id = (SELECT id FROM email_jobs WHERE status='pending' AND run_at <= NOW() LIMIT 1 FOR UPDATE SKIP LOCKED)
        ExecQuery --> [*]
    }

    Claiming --> Idle: ErrNoJobs (no pending jobs ready)
    Claiming --> Delivering: Job claimed

    state Delivering {
        [*] --> CheckoutSMTP: Borrow connection from pool
        CheckoutSMTP --> HealthCheck: NOOP command
        HealthCheck --> SendMail: MAIL FROM, RCPT TO, DATA
        SendMail --> [*]
    }

    Delivering --> Sent: SMTP 250 OK
    Delivering --> Retry: 4xx/5xx SMTP error / network drop

    state Sent {
        [*] --> MarkSent: UPDATE email_jobs SET status='sent', sent_at=NOW()
        MarkSent --> CheckBatch: Check if all batch jobs completed
        CheckBatch --> [*]
    }

    state Retry {
        [*] --> CheckAttempts: attempts < max_attempts?
        CheckAttempts --> Requeue: Yes: status='pending', run_at = NOW() + attempts*5s
        CheckAttempts --> FailPermanent: No: status='failed', last_error = err
        Requeue --> [*]
        FailPermanent --> [*]
    }

    Sent --> Idle: Return connection to SMTP pool
    Retry --> Idle: Discard broken connection & loop
```

