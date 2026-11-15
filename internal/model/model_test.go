package model

import "testing"

func TestNormalizeCode(t *testing.T) {
	tests := []struct {
		level Level
		in    string
		want  string
	}{
		{Province, "32", "32"},
		{Province, "32.73", ""}, // wrong shape for the level
		{Regency, "32.73", "32.73"},
		{Regency, " 32.73 ", "32.73"},
		{Regency, "3273", ""},
		{District, "32.73.01", "32.73.01"},
		{Village, "32.73.01.1001", "32.73.01.1001"},
		{Village, "BPS, prov=14, kota=71", ""},
		{Province, "JB", ""},
	}
	for _, tt := range tests {
		if got := NormalizeCode(tt.level, tt.in); got != tt.want {
			t.Errorf("NormalizeCode(%v, %q) = %q, want %q", tt.level, tt.in, got, tt.want)
		}
	}
}

func TestCodeHasPrefix(t *testing.T) {
	if !CodeHasPrefix("32.73.01", "32.73") {
		t.Error("32.73.01 is under 32.73")
	}
	if CodeHasPrefix("32.730.01", "32.73") {
		t.Error("32.730 must not match 32.73")
	}
	if CodeHasPrefix("32.73", "") {
		t.Error("empty parent code never matches")
	}
}

func TestDeriveKind(t *testing.T) {
	tests := []struct {
		name         string
		level        Level
		official, nm string
		code         string
		want         string
	}{
		{"official name wins", Regency, "Kota Bandung", "Bandung", "", "kota"},
		{"kabupaten administrasi", Regency, "Kabupaten Administrasi Kepulauan Seribu", "", "", "kabupaten"},
		{"falls back to name", Regency, "", "Kabupaten Bogor", "", "kabupaten"},
		{"kota by code", Regency, "", "Bandung", "32.73", "kota"},
		{"kabupaten by code", Regency, "", "Bandung", "32.04", "kabupaten"},
		{"unknown regency", Regency, "", "Bandung", "", ""},
		{"distrik in Papua", District, "Distrik Abepura", "Abepura", "", "distrik"},
		{"kecamatan", District, "Kecamatan Antapani", "", "", "kecamatan"},
		{"kelurahan by code", Village, "", "Antapani Kidul", "32.73.01.1001", "kelurahan"},
		{"desa by code", Village, "", "Sukamaju", "32.04.01.2001", "desa"},
		{"desa by name", Village, "Desa Sukamaju", "", "", "desa"},
		{"provinces have no kind", Province, "Provinsi Jawa Barat", "", "32", ""},
	}
	for _, tt := range tests {
		if got := DeriveKind(tt.level, tt.official, tt.nm, tt.code); got != tt.want {
			t.Errorf("%s: DeriveKind = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLevelNames(t *testing.T) {
	if Regency.Plural() != "regencies" || Village.Plural() != "villages" {
		t.Error("unexpected plural names")
	}
	if Level(8).Valid() || Level(3).Valid() {
		t.Error("only 4-7 are valid levels")
	}
}
