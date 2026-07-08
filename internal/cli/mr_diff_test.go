package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func diffMergeRequestJSON() string {
	return `{"iid":123,"title":"t","state":"opened","diff_refs":{"base_sha":"base000","head_sha":"head000","start_sha":"start000"}}`
}

func diffFilesJSON() string {
	return `[` +
		`{"old_path":"src/app.go","new_path":"src/app.go","diff":"@@ -10,4 +12,5 @@\n ctx1\n ctx2\n-rm1\n+add1\n+add2\n ctx3\n"},` +
		`{"old_path":"old/name.go","new_path":"new/name.go","renamed_file":true,"diff":"@@ -1,2 +1,2 @@\n ctx\n-x\n+y\n"}` +
		`]`
}

func TestMRDiffSummaryAxi(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case commentMRPath:
			fmt.Fprint(w, diffMergeRequestJSON())
		case commentDiffsPath:
			fmt.Fprint(w, diffFilesJSON())
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil,
		"mr", "diff", "123", "--fields", "old_path,new_ranges,old_ranges")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, fragment := range []string{
		"diff:",
		"base_sha: base000",
		"files[2]{path,status,additions,deletions,hunks,old_path,new_ranges,old_ranges}:",
		"src/app.go,modified,2,1,1,",
		"new/name.go,renamed,1,1,1,old/name.go",
		"count: 2 of 2 total",
		"mr diff export 123 --dir .gl-axi/mr-123 --project group/project",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, out)
		}
	}
}

func TestMRDiffSummaryFiltersFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case commentMRPath:
			fmt.Fprint(w, diffMergeRequestJSON())
		case commentDiffsPath:
			fmt.Fprint(w, diffFilesJSON())
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil,
		"mr", "diff", "123", "--file", "old/name.go", "--fields", "old_path")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "files[1]{path,status,additions,deletions,hunks,old_path}:") ||
		!strings.Contains(out, "new/name.go,renamed,1,1,1,old/name.go") ||
		!strings.Contains(out, "count: 1 of 1 total") {
		t.Fatalf("filtered output wrong:\n%s", out)
	}
}

func TestMRDiffPatchPrintsRawDiff(t *testing.T) {
	const rawPatch = "diff --git a/src/app.go b/src/app.go\n+hello\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != commentMRPath+"/raw_diffs" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, rawPatch)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "diff", "patch", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out != rawPatch {
		t.Fatalf("raw patch output = %q, want %q", out, rawPatch)
	}
}

func TestMRDiffExportWritesBundle(t *testing.T) {
	rawFiles := map[string]string{
		"base000:src/app.go":  "old app",
		"head000:src/app.go":  "new app",
		"base000:old/name.go": "old name",
		"head000:new/name.go": "new name",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.EscapedPath() == commentMRPath:
			fmt.Fprint(w, diffMergeRequestJSON())
		case r.URL.EscapedPath() == commentDiffsPath:
			fmt.Fprint(w, diffFilesJSON())
		case r.URL.EscapedPath() == commentMRPath+"/raw_diffs":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "diff --git a/src/app.go b/src/app.go\n")
		case r.URL.EscapedPath() == commentMRPath+"/versions":
			fmt.Fprint(w, `[{"id":9,"state":"collected","created_at":"2026-07-08T10:00:00Z"}]`)
		case strings.Contains(r.URL.EscapedPath(), "/repository/files/"):
			repoPath := rawFilePathFromRequest(t, r)
			key := r.URL.Query().Get("ref") + ":" + repoPath
			body, ok := rawFiles[key]
			if !ok {
				t.Errorf("unexpected raw file key %q", key)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, body)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	dir := filepath.Join(t.TempDir(), "bundle")
	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "diff", "export", "123", "--dir", dir)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "export:") || !strings.Contains(out, "files: 2") || !strings.Contains(out, "old_files: 2") || !strings.Contains(out, "new_files: 2") {
		t.Fatalf("export output wrong:\n%s", out)
	}

	assertFileContains(t, filepath.Join(dir, "manifest.toon"), "diff_version:\n  id: 9")
	assertFileContains(t, filepath.Join(dir, "files.toon"), "new/name.go")
	assertFileContains(t, filepath.Join(dir, "patch.diff"), "diff --git a/src/app.go b/src/app.go")
	assertFileContains(t, filepath.Join(dir, "diffs", "src", "app.go.diff"), "diff --git a/src/app.go b/src/app.go")
	assertFileContains(t, filepath.Join(dir, "old", "src", "app.go"), "old app")
	assertFileContains(t, filepath.Join(dir, "new", "new", "name.go"), "new name")
}

func TestMRDiffExportPathSafety(t *testing.T) {
	if _, err := safeRepoPath("../secret"); !errors.Is(err, errUnsafeExportPath) {
		t.Fatalf("safeRepoPath returned %v, want errUnsafeExportPath", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareMRDiffExportDir(dir, false); !errors.Is(err, errExportDirNotEmpty) {
		t.Fatalf("prepareMRDiffExportDir error = %v, want errExportDirNotEmpty", err)
	}
}

func rawFilePathFromRequest(t *testing.T, r *http.Request) string {
	t.Helper()
	parts := strings.Split(r.URL.EscapedPath(), "/repository/files/")
	if len(parts) != 2 {
		t.Fatalf("raw file path %q missing repository/files segment", r.URL.EscapedPath())
	}
	encoded := strings.TrimSuffix(parts[1], "/raw")
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		t.Fatalf("unescape raw file path: %v", err)
	}

	return decoded
}

func assertFileContains(t *testing.T, path string, fragment string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(body), fragment) {
		t.Fatalf("%s missing %q:\n%s", path, fragment, string(body))
	}
}
