# Frontend Pipeline & Lifecycle

This document details the current component lifecycle, state management, and data flow of the frontend application (`Vue 3` + `Vite` + `Tailwind CSS` + `@tanstack/vue-query`), specifically highlighting the file upload flow and the browser memory crash path.

---

## 1. High-Level Architecture & Component Map

```mermaid
flowchart TD
    App["App.vue (Shell Layout & Navigation)"] --> RouterView["Router View"]

    subgraph Views["Active Views"]
        VerifyView["VerifyEmailsView.vue (/verify)"]
        NewBatchView["NewBulkEmailView.vue (/bulk-emails/new)"]
        DetailView["BulkEmailDetailView.vue (/bulk-emails/:id)"]
    end

    RouterView --> VerifyView
    RouterView --> NewBatchView
    RouterView --> DetailView

    subgraph ComposablesAndStores["State & Business Logic"]
        CheckComp["useRecipientCheck()"]
        DraftComp["useBulkEmailDraft()"]
        PendingStore["usePendingCheckStore (Pinia)"]
        QueryCache["TanStack Vue Query Cache"]
    end

    VerifyView --> CheckComp
    VerifyView --> PendingStore
    NewBatchView --> DraftComp
    DraftComp --> CheckComp
    DraftComp --> PendingStore

    subgraph UIComponents["Reusable Wizard Components"]
        UploadComp["FileUpload.vue (Drop Zone)"]
        ReportComp["ReviewReport.vue (Summary & Table)"]
        EditorComp["MessageEditor.vue (Template Textareas)"]
        PreviewComp["EmailPreview.vue (Sample Carousel)"]
    end

    VerifyView --> UploadComp
    VerifyView --> ReportComp
    NewBatchView --> UploadComp
    NewBatchView --> ReportComp
    NewBatchView --> EditorComp
    NewBatchView --> PreviewComp
```

---

## 2. File Upload & Verification Lifecycle (`VerifyEmailsView`)

This diagram shows what happens from the moment the user selects a file until the result is rendered.

```mermaid
sequenceDiagram
    autonumber
    actor User as User (Browser)
    participant UI as FileUpload.vue
    participant Composable as useRecipientCheck.ts
    participant API as api/recipients.ts (parseRecipients)
    participant Axios as Axios / Fetch Client
    participant V8 as Chrome V8 JS Engine
    participant Report as ReviewReport.vue
    participant DOM as Chrome Blink DOM & Renderer

    User->>UI: Drops or selects file (1M rows .xlsx)
    UI->>Composable: chooseFile(file)
    Composable->>Composable: fileProblem(): Check extension (.xlsx) and size <= MAX_UPLOAD_MB
    Composable->>API: parse.mutate(file)
    API->>Axios: POST /api/v1/recipients/parse (FormData)
    Note over Axios: Uploads file to Go backend (~18MB payload)

    Axios-->>V8: Receives HTTP 200 OK with 78MB - 100MB JSON payload
    
    rect rgb(255, 235, 235)
    Note over V8,Composable: Memory Explosion: JSON Parsing into V8 Heap
    V8->>V8: JSON.parse() parses 1M objects
    Note over V8: JS Heap expands 8x-12x: ~600MB - 1GB heap allocation!
    V8-->>Composable: onSuccess(checked: ParseResult)
    Composable->>Composable: result.value = checked (shallowRef)
    end

    Composable->>Report: Props: result (valid: 800k items, problems: 170k items)
    
    rect rgb(255, 220, 220)
    Note over Report,DOM: THE CRASH POINT: Unvirtualized DOM Rendering
    Report->>Report: tabs computed: runs .filter() 4x over 170k items
    Report->>DOM: Render <table> with v-for="row in rows"
    Note over DOM: Vue creates ~1.5 million Virtual DOM VNodes
    DOM->>DOM: Chrome attempts to create 170,000 <tr> and 680,000 <td> nodes
    DOM->>DOM: Style recalculation & Layout tree computation
    Note over DOM: Browser tab exceeds 2GB-4GB renderer memory limit
    DOM-->>User: 💥 Chrome Tab Crash: "Aw, Snap! Out of Memory"
    end
```

---

## 3. Data Flow Between Verify and New Bulk Email

The application allows users to verify a file and then carry the parsed result over to the bulk email creation wizard without re-uploading:

```mermaid
flowchart LR
    subgraph VerifyPage["VerifyEmailsView.vue"]
        V_File["check.file"]
        V_Result["check.result"]
        V_Btn["Continue to New Bulk Email"]
    end

    subgraph Store["Pinia: usePendingCheckStore"]
        StoreHeld["pending = { file, result }"]
    end

    subgraph NewBatchPage["NewBulkEmailView.vue"]
        Adopt["draft.adopt(file, result)"]
        D_File["draft.file"]
        D_Result["draft.result"]
        D_Preview["Sample Preview (Rows 0..4)"]
        D_Editor["Subject & Body Editor"]
    end

    V_Btn -->|"pending.hold(file, result)"| StoreHeld
    StoreHeld -->|"pending.take()"| Adopt
    Adopt --> D_File
    Adopt --> D_Result
    D_Result --> D_Preview
    D_Result --> D_Editor
```

---

## 4. Batch Creation & Progress Monitoring Lifecycle

When creating a batch from `NewBulkEmailView.vue` through sending and live tracking:

```mermaid
sequenceDiagram
    autonumber
    actor User as User
    participant View as NewBulkEmailView.vue
    participant Draft as useBulkEmailDraft.ts
    participant API as api/recipients.ts (createBatch)
    participant Router as Vue Router
    participant Detail as BulkEmailDetailView.vue
    participant Query as @tanstack/vue-query (useBatchProgress)

    User->>View: Clicks "Send Bulk Email"
    View->>View: Opens ConfirmDialog ("X valid recipients will receive emails...")
    User->>View: Confirms modal
    View->>Draft: draft.send()
    Draft->>API: createBatch({ file, subject, body })
    API->>API: POST /api/v1/batches
    Note over API: Backend inserts jobs and returns 202 Accepted
    API-->>Draft: 202 Accepted { batch_id: 42, valid_count: ... }
    
    Draft-->>View: Batch created successfully
    View->>Router: router.push('/bulk-emails/42')
    Router->>Detail: Mount BulkEmailDetailView (id = 42)

    rect rgb(240, 248, 255)
    Note over Detail,Query: Live Progress Polling Lifecycle
    loop Every 3 seconds (refetchInterval)
        Detail->>Query: fetchBatchProgress(42)
        Query->>Query: GET /api/v1/batches/42
        Query-->>Detail: { sent, failed, pending, processing, invalid }
        Detail->>Detail: Update KPI tiles & progress bar percentage
    end
    Note over Detail: Polling stops automatically when status == 'completed'
    end
```

---

## 5. UI Bottlenecks & Failure Summary

```mermaid
flowchart TD
    subgraph RootCauses["Root Causes of Frontend Failure on 1M Rows"]
        R1["Massive Network Payload<br/>(78MB-100MB JSON)"]
        R2["V8 Memory Amplification<br/>(1M distinct objects = ~1GB heap)"]
        R3["Repeated Array Filtering<br/>(.filter() on 170k objects in computed)"]
        R4["Direct Non-Virtual DOM Table<br/>(170,000 tr tags rendered simultaneously)"]
    end

    subgraph Consequences["Browser Consequences"]
        C1["Network thread stalls & high download latency"]
        C2["V8 GC thrashing & tab freeze"]
        C3["Chrome Layout Engine OOM"]
    end

    subgraph Crash["Terminal Outcome"]
        OOM["Renderer Process Killed<br/>'Aw, Snap! Error code: Out of Memory'"]
    end

    R1 --> C1
    R2 --> C2
    R3 --> C2
    R4 --> C3
    C1 --> OOM
    C2 --> OOM
    C3 --> OOM
```

