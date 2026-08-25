package logging

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
