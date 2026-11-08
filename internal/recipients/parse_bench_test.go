package recipients

import (
	"os"
	"runtime"
	"runtime/metrics"
	"sync"
	"testing"
	"time"
)

// BenchmarkParseExcelReader measures parsing one big spreadsheet, the same way
// the API does (ParseExcelReader on the uploaded file).
//
// It is skipped unless BENCH_XLSX names a file, so `go test ./...` stays fast:
//
//	BENCH_XLSX=$PWD/samples/12_million_mixed.xlsx \
//	  go test ./internal/recipients -run '^$' -bench ParseExcelReader -benchtime 1x -count 6
//
// Besides time (ns/op), memory allocated (B/op) and allocations (allocs/op)
// from -benchmem, it reports:
//   - peak-heap-MB: the most heap memory in use at any moment during one parse,
//     above what was in use before it started. This is what decides whether the
//     server runs out of memory, so it is the main number to compare.
//   - valid-rows / problem-rows: a sanity check that both versions parsed the
//     same file the same way.
func BenchmarkParseExcelReader(b *testing.B) {
	path := os.Getenv("BENCH_XLSX")
	if path == "" {
		b.Skip("set BENCH_XLSX to a .xlsx file to run this benchmark")
	}

	b.ReportAllocs()
	var peakMB float64
	var valid, problems int

	for b.Loop() {
		b.StopTimer()
		f, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		runtime.GC() // start every run from the same clean heap
		stop := watchPeakHeap()
		b.StartTimer()

		v, p, err := ParseExcelReader(f)

		b.StopTimer()
		peak := stop()
		f.Close()
		if err != nil {
			b.Fatal(err)
		}
		valid, problems = len(v), len(p)
		peakMB = max(peakMB, float64(peak)/(1<<20))
		b.StartTimer()
	}

	b.ReportMetric(peakMB, "peak-heap-MB")
	b.ReportMetric(float64(valid), "valid-rows")
	b.ReportMetric(float64(problems), "problem-rows")
}

// watchPeakHeap samples live heap memory every few milliseconds until the
// returned stop function is called, which reports the highest value seen above
// the starting point. runtime/metrics is used because, unlike
// runtime.ReadMemStats, reading it does not pause the program.
func watchPeakHeap() (stop func() uint64) {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	read := func() uint64 {
		metrics.Read(sample)
		return sample[0].Value.Uint64()
	}

	base := read()
	peak := base
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				peak = max(peak, read())
			}
		}
	}()

	return func() uint64 {
		close(done)
		wg.Wait()
		peak = max(peak, read())
		return peak - base
	}
}
