package main

import (
	"testing"
	"time"
)

func TestComputeTimeRange_UTC_SingleDay(t *testing.T) {
	startMs, endMs, err := computeTimeRange("20241001", "20241001", time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantStart := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	wantEnd := time.Date(2024, 10, 1, 23, 59, 59, 999_000_000, time.UTC).UnixMilli()

	if startMs != wantStart {
		t.Errorf("startMs = %d, want %d", startMs, wantStart)
	}
	if endMs != wantEnd {
		t.Errorf("endMs = %d, want %d", endMs, wantEnd)
	}
}

func TestComputeTimeRange_FixedZone_JST(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)

	startMs, endMs, err := computeTimeRange("20241001", "20241001", jst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantStart := time.Date(2024, 10, 1, 0, 0, 0, 0, jst).UnixMilli()
	wantEnd := time.Date(2024, 10, 1, 23, 59, 59, 999_000_000, jst).UnixMilli()

	if startMs != wantStart {
		t.Errorf("startMs = %d, want %d", startMs, wantStart)
	}
	if endMs != wantEnd {
		t.Errorf("endMs = %d, want %d", endMs, wantEnd)
	}
}

func TestComputeTimeRange_DSTSpringForward(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata not available: %v", err)
	}

	// 2024-03-10 in America/New_York: 02:00 jumps to 03:00 (spring forward).
	// The wall-clock end of the day is 23:59:59.999 EDT (UTC-04:00),
	// which equals 2024-03-11T03:59:59.999Z.
	_, endMs, err := computeTimeRange("20240310", "20240310", ny)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2024, 3, 11, 3, 59, 59, 999_000_000, time.UTC).UnixMilli()
	if endMs != want {
		t.Errorf("endMs = %d, want %d (wall-clock 2024-03-10 23:59:59.999 in America/New_York after DST)", endMs, want)
	}

	// The previous implementation used endDate.Add(23h59m59s), which would have
	// produced 2024-03-11T04:59:59.000Z (one hour late, ms truncated). Guard
	// against regressing back to that behavior.
	regression := time.Date(2024, 3, 10, 0, 0, 0, 0, ny).Add(23*time.Hour + 59*time.Minute + 59*time.Second).UnixMilli()
	if endMs == regression {
		t.Errorf("endMs matches the buggy Add-based computation (%d); DST handling regressed", regression)
	}
}

func TestComputeTimeRange_DSTFallBack(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata not available: %v", err)
	}

	// 2024-11-03 in America/New_York: 02:00 falls back to 01:00.
	// Wall-clock end of day is 23:59:59.999 EST (UTC-05:00),
	// which equals 2024-11-04T04:59:59.999Z.
	_, endMs, err := computeTimeRange("20241103", "20241103", ny)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := time.Date(2024, 11, 4, 4, 59, 59, 999_000_000, time.UTC).UnixMilli()
	if endMs != want {
		t.Errorf("endMs = %d, want %d (wall-clock 2024-11-03 23:59:59.999 in America/New_York after fall-back)", endMs, want)
	}
}

func TestComputeTimeRange_MultiDay(t *testing.T) {
	startMs, endMs, err := computeTimeRange("20241001", "20241031", time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantStart := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	wantEnd := time.Date(2024, 10, 31, 23, 59, 59, 999_000_000, time.UTC).UnixMilli()

	if startMs != wantStart {
		t.Errorf("startMs = %d, want %d", startMs, wantStart)
	}
	if endMs != wantEnd {
		t.Errorf("endMs = %d, want %d", endMs, wantEnd)
	}
}

func TestNormalizeNewlines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no newline", "abc", "abc"},
		{"already LF", "abc\ndef", "abc\ndef"},
		{"CRLF", "abc\r\ndef", "abc\ndef"},
		{"bare CR (classic Mac)", "abc\rdef", "abc\ndef"},
		{"trailing CRLF", "abc\r\n", "abc\n"},
		{"trailing CR", "abc\r", "abc\n"},
		{"only CRLF", "\r\n", "\n"},
		{"only CR", "\r", "\n"},
		{"mixed CRLF and bare CR", "a\r\nb\rc", "a\nb\nc"},
		{"adjacent CR then LF must not become two LFs", "a\r\nb", "a\nb"},
		{"two CRLF", "a\r\n\r\nb", "a\n\nb"},
		{"CR followed by another CR", "a\r\rb", "a\n\nb"},
		{"multibyte UTF-8 around CRLF", "日本語\r\n改行\rテスト", "日本語\n改行\nテスト"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeNewlines(tc.in)
			if got != tc.want {
				t.Errorf("normalizeNewlines(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestComputeTimeRange_InvalidDate(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
	}{
		{"non-numeric start", "BAD", "20241031"},
		{"non-numeric end", "20241001", "BAD"},
		{"non-existent date", "20240230", "20240301"},
		{"hyphenated format", "2024-10-01", "20241031"},
		{"empty start", "", "20241031"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := computeTimeRange(tc.start, tc.end, time.UTC)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
