package fetch

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// server mimics Geofabrik: a dated file, its .md5, and a -latest redirect.
func server(t *testing.T, body string, md5sum string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var downloads atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/asia/indonesia-latest.osm.pbf", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/asia/indonesia-261114.osm.pbf", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/asia/indonesia-261114.osm.pbf.md5", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  indonesia-261114.osm.pbf\n", md5sum)
	})
	mux.HandleFunc("/asia/indonesia-261114.osm.pbf", func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		http.ServeContent(w, r, "x.pbf", time.Time{}, strings.NewReader(body)) // honours Range
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &downloads
}

func sum(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestGetResolvesLatestDownloadsAndVerifies(t *testing.T) {
	body := strings.Repeat("osm-data-", 1000)
	srv, downloads := server(t, body, sum(body))
	dir := t.TempDir()

	res, err := Get(context.Background(), Options{BaseURL: srv.URL, Region: "asia/indonesia", Dir: dir}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(res.Path) != "indonesia-261114.osm.pbf" || !res.Downloaded {
		t.Errorf("unexpected result: %+v", res)
	}
	if got, _ := os.ReadFile(res.Path); string(got) != body {
		t.Error("downloaded content differs")
	}
	if si, ok := ReadSidecar(res.Path); !ok || si.URL != res.URL || si.MD5 != sum(body) {
		t.Errorf("sidecar = %+v, %v", si, ok)
	}

	// A second call finds the verified file and does not download again.
	res, err = Get(context.Background(), Options{BaseURL: srv.URL, Region: "asia/indonesia", Dir: dir}, quiet)
	if err != nil || res.Downloaded {
		t.Fatalf("second call: %+v, %v", res, err)
	}
	if n := downloads.Load(); n != 1 {
		t.Errorf("file was downloaded %d times, want 1", n)
	}
}

func TestGetRejectsChecksumMismatchAndCleansUp(t *testing.T) {
	srv, _ := server(t, "corrupted", sum("what geofabrik thinks it is"))
	dir := t.TempDir()

	_, err := Get(context.Background(), Options{BaseURL: srv.URL, Region: "asia/indonesia", Snapshot: "261114", Dir: dir}, quiet)
	if err == nil || !strings.Contains(err.Error(), "md5") {
		t.Fatalf("expected an md5 error, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("leftover files after a failed download: %v", entries)
	}
}

func TestGetResumesAPartialDownload(t *testing.T) {
	body := strings.Repeat("0123456789", 500)
	srv, _ := server(t, body, sum(body))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "indonesia-261114.osm.pbf.part"), []byte(body[:1234]), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Get(context.Background(), Options{BaseURL: srv.URL, Region: "asia/indonesia", Snapshot: "261114", Dir: dir}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(res.Path); string(got) != body {
		t.Error("resumed file differs from the original")
	}
}

func TestGetFailsClearlyWhenLatestDoesNotRedirect(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := Get(context.Background(), Options{BaseURL: srv.URL, Region: "asia/indonesia", Dir: t.TempDir()}, quiet)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Errorf("err = %v", err)
	}
}
