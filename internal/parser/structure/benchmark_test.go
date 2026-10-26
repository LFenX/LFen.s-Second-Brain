package structure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The fixed benchmark has eight pre-existing synthetic files padded with comments
// or paragraphs. This measures only the pure producer; it does not establish the
// end-to-end task/API/RSS acceptance that the integrating session must measure.
func TestFrozenSyntheticBenchmark(t *testing.T) {
	root := os.Getenv("LFEN_STRUCTURE_BENCHMARK_ROOT")
	if root == "" {
		t.Skip("explicit synthetic benchmark path not supplied")
	}
	paths := make([]string, 0, len(benchmarkSHA))
	for path := range benchmarkSHA {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	inputs := make([]Input, 0, len(paths))
	total := 0
	for i, path := range paths {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if digest(body) != benchmarkSHA[path] {
			t.Fatalf("fixed benchmark SHA mismatch: %s", path)
		}
		parser := "typescript"
		switch filepath.Ext(path) {
		case ".tsx":
			parser = "tsx"
		case ".go":
			parser = "go"
		case ".md":
			parser = "markdown"
		}
		inputs = append(inputs, Input{SourceID: fmt.Sprintf("%032x", i+1), RevisionID: fmt.Sprintf("%032x", i+101), Path: path, Parser: parser, Content: body})
		total += len(body)
	}
	if len(inputs) != 8 || total != 982755 {
		t.Fatal("fixed synthetic benchmark shape changed")
	}
	before := linuxRSS()
	for run := 0; run < 5; run++ {
		start := time.Now()
		output, err := Build(context.Background(), inputs, []Alias{{Prefix: "@/", Target: "frontend/"}})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		nodes, edges := 0, 0
		for _, file := range output.Files {
			nodes += len(file.Nodes)
			edges += len(file.Relations) + len(file.Nodes) - 1
		}
		t.Logf("producer only run=%d bytes=%d nodes=%d relations_with_contains=%d duration=%s rss_before=%d rss_sample=%d", run+1, total, nodes, edges, elapsed, before, linuxRSS())
		if elapsed > 10*time.Second {
			t.Fatal("pure producer exceeds the full-task time budget; investigate before integration")
		}
	}
}

func linuxRSS() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return value * 1024
				}
			}
		}
	}
	return -1
}
