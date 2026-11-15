// Package fetch downloads a dated OpenStreetMap extract from Geofabrik and
// verifies it against the checksum Geofabrik publishes next to it.
package fetch

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Base is the Geofabrik download server.
const Base = "https://download.geofabrik.de"

// Options select what to download.
type Options struct {
	BaseURL string // default Base
	Region  string // path on the server, e.g. "asia/indonesia"
	// Snapshot is "latest", or the yymmdd of a dated file ("261114").
	// Geofabrik keeps dated files only for a limited time.
	Snapshot string
	Dir      string // where the file is stored
	Client   *http.Client
	// UserAgent is sent with every request. Defaults to "toi".
	UserAgent string
}

// Result describes the verified file.
type Result struct {
	Path       string // local file
	URL        string // where it came from
	MD5        string
	Downloaded bool // false when a verified copy was already present
}

// Get makes sure a verified copy of the extract is in opt.Dir.
func Get(ctx context.Context, opt Options, log *slog.Logger) (*Result, error) {
	if opt.BaseURL == "" {
		opt.BaseURL = Base
	}
	if opt.Snapshot == "" {
		opt.Snapshot = "latest"
	}
	if opt.Client == nil {
		opt.Client = &http.Client{}
	}
	if opt.UserAgent == "" {
		opt.UserAgent = "toi"
	}
	if opt.Region == "" || opt.Dir == "" {
		return nil, errors.New("fetch: region and dir are required")
	}
	c := &client{opt: opt}

	name, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	fileURL := strings.TrimRight(opt.BaseURL, "/") + "/" + path.Dir(opt.Region) + "/" + name
	if path.Dir(opt.Region) == "." {
		fileURL = strings.TrimRight(opt.BaseURL, "/") + "/" + name
	}
	want, err := c.fetchMD5(ctx, fileURL+".md5")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(opt.Dir, name)

	if got, err := fileMD5(dest); err == nil && got == want {
		log.Info("already downloaded and verified", "file", dest)
		return &Result{Path: dest, URL: fileURL, MD5: want}, writeSidecar(dest, fileURL, want)
	}
	if err := c.download(ctx, fileURL, dest+".part", log); err != nil {
		return nil, err
	}
	got, err := fileMD5(dest + ".part")
	if err != nil {
		return nil, err
	}
	if got != want {
		os.Remove(dest + ".part")
		return nil, fmt.Errorf("fetch: md5 of %s is %s, Geofabrik says %s; removed the partial file", name, got, want)
	}
	if err := os.Rename(dest+".part", dest); err != nil {
		return nil, err
	}
	return &Result{Path: dest, URL: fileURL, MD5: want, Downloaded: true}, writeSidecar(dest, fileURL, want)
}

// SourceInfo is stored next to a downloaded file so that later steps, which
// only get the path, can record where it came from.
type SourceInfo struct {
	URL string `json:"url"`
	MD5 string `json:"md5"`
}

// SidecarPath is where the SourceInfo of file lives.
func SidecarPath(file string) string { return file + ".source.json" }

func writeSidecar(file, url, md5sum string) error {
	b, err := json.Marshal(SourceInfo{URL: url, MD5: md5sum})
	if err != nil {
		return err
	}
	return os.WriteFile(SidecarPath(file), append(b, '\n'), 0o644)
}

// ReadSidecar returns the SourceInfo stored next to file, if there is one.
func ReadSidecar(file string) (SourceInfo, bool) {
	b, err := os.ReadFile(SidecarPath(file))
	if err != nil {
		return SourceInfo{}, false
	}
	var si SourceInfo
	return si, json.Unmarshal(b, &si) == nil
}

type client struct{ opt Options }

func (c *client) do(ctx context.Context, method, u string, hdr map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.opt.UserAgent)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return c.opt.Client.Do(req)
}

// resolve returns the file name to download, turning "latest" into the dated
// file Geofabrik currently redirects to.
func (c *client) resolve(ctx context.Context) (string, error) {
	base := path.Base(c.opt.Region)
	if c.opt.Snapshot != "latest" {
		return base + "-" + c.opt.Snapshot + ".osm.pbf", nil
	}
	u := strings.TrimRight(c.opt.BaseURL, "/") + "/" + c.opt.Region + "-latest.osm.pbf"
	noRedirect := *c.opt.Client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	cc := &client{opt: c.opt}
	cc.opt.Client = &noRedirect
	resp, err := cc.do(ctx, http.MethodHead, u, nil)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || loc == "" {
		return "", fmt.Errorf("fetch: expected %s to redirect to a dated file, got HTTP %d", u, resp.StatusCode)
	}
	lu, err := url.Parse(loc)
	if err != nil {
		return "", err
	}
	return path.Base(lu.Path), nil
}

func (c *client) fetchMD5(ctx context.Context, u string) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch: %s: HTTP %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	sum, _, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	if len(sum) != 32 {
		return "", fmt.Errorf("fetch: %s does not contain an md5 sum", u)
	}
	return strings.ToLower(sum), nil
}

// download fetches u into part, resuming a previous partial download.
func (c *client) download(ctx context.Context, u, part string, log *slog.Logger) error {
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
	}
	hdr := map[string]string{}
	if offset > 0 {
		hdr["Range"] = fmt.Sprintf("bytes=%d-", offset)
	}
	resp, err := c.do(ctx, http.MethodGet, u, hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
		log.Info("resuming download", "from_bytes", offset)
	case http.StatusOK:
		flags |= os.O_TRUNC // the server ignored Range, start over
	case http.StatusRequestedRangeNotSatisfiable:
		return nil // the partial file already has every byte; the checksum decides
	default:
		return fmt.Errorf("fetch: %s: HTTP %d", u, resp.StatusCode)
	}
	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	pr := &progress{r: resp.Body, log: log, total: resp.ContentLength, last: time.Now()}
	if _, err := io.Copy(f, pr); err != nil {
		f.Close()
		return fmt.Errorf("fetch: downloading %s: %w", u, err)
	}
	return f.Close()
}

type progress struct {
	r     io.Reader
	log   *slog.Logger
	total int64
	done  int64
	last  time.Time
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if time.Since(p.last) > 15*time.Second {
		p.last = time.Now()
		p.log.Info("downloading", "mb", p.done>>20, "of_mb", p.total>>20)
	}
	return n, err
}

func fileMD5(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
