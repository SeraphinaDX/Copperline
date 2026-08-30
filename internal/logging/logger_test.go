package logging

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"copperline/internal/model"
)

func TestTailLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := tailLines(f, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"three", "four", "five"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tailLines() = %#v, want %#v", got, want)
	}
}

func TestLoggerTailUsesBufferPath(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(serverDir, "#copperline.log")
	if err := os.WriteFile(path, []byte("old\nnewer\nnewest\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	l := New(true, dir, "15:04")
	got, err := l.Tail("libera", "#copperline", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"newer", "newest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tail() = %#v, want %#v", got, want)
	}
}

func TestBacklogTailStopsAtCurrentSessionBoundary(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(serverDir, "#later.log")
	old := "2026-08-26 12:00:00 <alice> yesterday one\n" +
		"2026-08-26 12:01:00 <bob> yesterday two\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	l := New(true, dir, "2006-01-02 15:04:05")
	if err := l.Write(model.Message{
		Time:   time.Date(2026, 8, 27, 20, 0, 0, 0, time.Local),
		Server: "libera",
		Target: "#later",
		Nick:   "carol",
		Text:   "arrived before first view",
		Kind:   model.KindMessage,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := l.BacklogTail("libera", "#later", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"2026-08-26 12:00:00 <alice> yesterday one",
		"2026-08-26 12:01:00 <bob> yesterday two",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BacklogTail() = %#v, want only pre-session context %#v", got, want)
	}

	tail, err := l.Tail("libera", "#later", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 1 || tail[0] != "2026-08-27 20:00:00 <carol> arrived before first view" {
		t.Fatalf("Tail() = %#v, want current-session line to still be logged normally", tail)
	}
}

func TestBacklogTailSkipsLegacyHousekeeping(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(serverDir, "#copperline.log")
	data := "" +
		"2026-08-24 17:58:39 <alice> useful one\n" +
		"2026-08-24 17:58:40 *** 328 me #copperline https://example.invalid/\n" +
		"2026-08-24 17:58:41 *** 333 me #copperline setter 123\n" +
		"2026-08-24 17:58:41 *** 366 me #copperline End of /NAMES list.\n" +
		"2026-08-24 17:58:42 *** 352 me #copperline user host server nick H :0 Real Name\n" +
		"2026-08-24 17:58:42 *** 315 me #copperline End of /WHO list.\n" +
		"2026-08-24 17:58:43 *** 324 me #copperline +nt\n" +
		"2026-08-24 17:58:43 *** 329 me #copperline 123\n" +
		"2026-08-24 17:59:00 <bob> useful two\n" +
		"2026-08-24 17:59:01 <carol> useful three\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	l := New(true, dir, "15:04")
	got, err := l.BacklogTail("libera", "#copperline", 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"2026-08-24 17:58:39 <alice> useful one",
		"2026-08-24 17:59:00 <bob> useful two",
		"2026-08-24 17:59:01 <carol> useful three",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BacklogTail() = %#v, want %#v", got, want)
	}
}

func TestBacklogTailSearchesPastLargeNoiseBlock(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(serverDir, "#busy.log")
	var data string
	data += "2026-08-24 17:00:00 <alice> older useful\n"
	for i := 0; i < 100; i++ {
		data += "2026-08-24 17:01:00 *** 352 me #busy user host server nick H :0 Real Name\n"
	}
	data += "2026-08-24 17:02:00 <bob> newest useful\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	l := New(true, dir, "15:04")
	got, err := l.BacklogTail("libera", "#busy", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"2026-08-24 17:00:00 <alice> older useful",
		"2026-08-24 17:02:00 <bob> newest useful",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BacklogTail() = %#v, want %#v", got, want)
	}
}
