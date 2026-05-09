package main

import (
	"encoding/json"
	"strings"
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

func TestEnsureTrailingNewline(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty becomes single LF (visible blank line)", "", "\n"},
		{"single LF unchanged", "\n", "\n"},
		{"multiple LFs compact to single LF", "\n\n\n", "\n"},
		{"no newline gets one", "abc", "abc\n"},
		{"already single LF terminated unchanged", "abc\n", "abc\n"},
		{"trailing double LF compacts to single LF", "abc\n\n", "abc\n"},
		{"trailing many LFs compact to single LF", "abc\n\n\n\n", "abc\n"},
		{"only-CR is preserved (CR is not stripped)", "abc\r", "abc\r\n"},
		{"multiple internal LFs, no trailing → adds one", "a\nb\nc", "a\nb\nc\n"},
		{"multiple internal LFs, single trailing → unchanged", "a\nb\nc\n", "a\nb\nc\n"},
		{"internal LFs preserved, trailing run compacted", "a\n\nb\n\n", "a\n\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureTrailingNewline(tc.in)
			if got != tc.want {
				t.Errorf("ensureTrailingNewline(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeAndEnsureNewline_Composition(t *testing.T) {
	// Verify the composition order used in main: normalizeNewlines first,
	// then ensureTrailingNewline. This ensures \r\n / \r terminated messages
	// don't end up double-newlined.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text needs newline", "hello", "hello\n"},
		{"already LF terminated", "hello\n", "hello\n"},
		{"CRLF terminated normalizes to LF, no doubling", "hello\r\n", "hello\n"},
		{"bare CR terminated normalizes to LF, no doubling", "hello\r", "hello\n"},
		{"internal CRLF and missing trailing", "a\r\nb", "a\nb\n"},
		{"trailing CRLF+LF run compacts to single LF (Docker-build style)", "Sending build context\r\n\n", "Sending build context\n"},
		{"empty input becomes a single LF (blank event stays visible)", "", "\n"},
		{"CRLF-only event becomes a single LF", "\r\n", "\n"},
		{"multiple CRLFs compact to a single LF", "\r\n\r\n", "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureTrailingNewline(normalizeNewlines(tc.in))
			if got != tc.want {
				t.Errorf("ensureTrailingNewline(normalizeNewlines(%q)) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestComputeTimeRange_HyphenatedDate(t *testing.T) {
	startMs, endMs, err := computeTimeRange("2024-10-01", "2024-10-01", time.UTC)
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

func TestComputeTimeRange_SecondPrecision(t *testing.T) {
	startMs, endMs, err := computeTimeRange("2024-10-01T12:30:00", "2024-10-01T12:30:59", time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2024, 10, 1, 12, 30, 0, 0, time.UTC).UnixMilli()
	wantEnd := time.Date(2024, 10, 1, 12, 30, 59, 999_000_000, time.UTC).UnixMilli()
	if startMs != wantStart {
		t.Errorf("startMs = %d, want %d", startMs, wantStart)
	}
	if endMs != wantEnd {
		t.Errorf("endMs = %d, want %d", endMs, wantEnd)
	}
}

func TestComputeTimeRange_MixedFormats(t *testing.T) {
	// start = bare date (granularity Day), end = second precision.
	startMs, endMs, err := computeTimeRange("20241001", "2024-10-01T12:00:00", time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	wantEnd := time.Date(2024, 10, 1, 12, 0, 0, 999_000_000, time.UTC).UnixMilli()
	if startMs != wantStart {
		t.Errorf("startMs = %d, want %d", startMs, wantStart)
	}
	if endMs != wantEnd {
		t.Errorf("endMs = %d, want %d", endMs, wantEnd)
	}
}

func TestComputeTimeRange_SecondPrecision_DSTSpringForward(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata not available: %v", err)
	}

	// 01:30 EST → 03:30 EDT spans only 1 hour of real time across the
	// spring-forward boundary. Verify the millisecond delta reflects that.
	startMs, endMs, err := computeTimeRange("2024-03-10T01:30:00", "2024-03-10T03:30:00", ny)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// end is the last ms of 03:30:00, so end - start == 3600s + 999ms - 1ms = 3600999.
	gotDelta := endMs - startMs
	wantDelta := int64(3600*1000 + 999)
	if gotDelta != wantDelta {
		t.Errorf("delta = %d ms, want %d ms (1 real hour across DST plus 999 ms tail)", gotDelta, wantDelta)
	}
}

func TestComputeTimeRange_InvertedRange(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
	}{
		{"day granularity, end before start", "20241031", "20241001"},
		{"second granularity, end before start", "2024-10-01T12:00:00", "2024-10-01T11:00:00"},
		{"mixed granularity inverted", "2024-10-02", "2024-10-01T23:59:59"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := computeTimeRange(tc.start, tc.end, time.UTC)
			if err == nil {
				t.Errorf("expected inverted-range error, got nil")
			}
		})
	}
}

func TestEndOfGranularity_UnknownPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for unknown granularity, got none")
		}
	}()
	endOfGranularity(time.Now(), granularity(999), time.UTC)
}

func TestComputeTimeRange_InvalidFormat(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
	}{
		{"non-numeric start", "BAD", "20241031"},
		{"non-numeric end", "20241001", "BAD"},
		{"non-existent date YYYYMMDD", "20240230", "20240301"},
		{"non-existent date YYYY-MM-DD", "2024-02-30", "2024-03-01"},
		{"empty start", "", "20241031"},
		{"slash separator", "2024/10/01", "20241031"},
		{"9-char unsupported length", "2024-10-1", "20241031"},
		{"minute-only precision (16 chars)", "2024-10-01T12:34", "20241031"},
		{"trailing Z (20 chars)", "2024-10-01T12:34:56Z", "20241031"},
		{"trailing offset (25 chars)", "2024-10-01T12:34:56+09:00", "20241031"},
		{"compact 14-char datetime not supported", "20241001120000", "20241031"},
		{"8 spaces (correct length, wrong content)", "        ", "20241031"},
		{"10 letters (correct length, wrong content)", "abcdefghij", "20241031"},
		{"fullwidth digits inflate byte length", "２０２４-10-01", "20241031"},
		{"en-dash separators inflate byte length", "2024–10–01", "20241031"},
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

func TestFormatTimestamp(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	// 2024-10-01T12:34:56.789Z, in three zones with various ms values.
	ms := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()

	cases := []struct {
		name string
		ms   int64
		loc  *time.Location
		want string
	}{
		{"UTC ends with Z", ms, time.UTC, "2024-10-01T12:34:56.789Z"},
		{"JST shows +09:00 offset", ms, jst, "2024-10-01T21:34:56.789+09:00"},
		{"millisecond zero pads to .000", time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), time.UTC, "2024-10-01T00:00:00.000Z"},
		{"millisecond .005 pads to .005", time.Date(2024, 10, 1, 0, 0, 0, 5_000_000, time.UTC).UnixMilli(), time.UTC, "2024-10-01T00:00:00.005Z"},
		{"millisecond .999 stays .999", time.Date(2024, 10, 1, 23, 59, 59, 999_000_000, time.UTC).UnixMilli(), time.UTC, "2024-10-01T23:59:59.999Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTimestamp(tc.ms, tc.loc)
			if got != tc.want {
				t.Errorf("formatTimestamp(%d, %s) = %q, want %q", tc.ms, tc.loc, got, tc.want)
			}
		})
	}
}

func TestFormatEvent_Raw(t *testing.T) {
	out, err := formatEvent(0, "hello\n", "ignored-stream", formatRaw, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello\n" {
		t.Errorf("got %q, want %q", out, "hello\n")
	}
}

func TestFormatEvent_WithTime(t *testing.T) {
	ts := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "hello\n", "ignored-stream", formatWithTime, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2024-10-01T12:34:56.789Z\thello\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFormatEvent_WithTime_EmptyMessage(t *testing.T) {
	// A blank-message event still carries timestamp information, so with-time
	// emits "<ts>\t\n" rather than dropping the event. The trailing \n is
	// essential — without it the next event would share the same line.
	ts := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "", "ignored-stream", formatWithTime, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2024-10-01T12:34:56.789Z\t\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFormatEvent_WithTime_PreservesMultilineMessage(t *testing.T) {
	// Multiline messages flow through as-is: only the first line gets a
	// timestamp prefix; subsequent lines appear verbatim. This is the
	// intentional design (see README); lock it in so a future "prefix every
	// line" change doesn't sneak in.
	ts := time.Date(2024, 10, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	out, _ := formatEvent(ts, "line1\nline2\n", "s", formatWithTime, time.UTC)
	want := "2024-10-01T12:00:00.000Z\tline1\nline2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFormatEvent_JSONL(t *testing.T) {
	ts := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "hello\n", "my-stream", formatJSONL, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("jsonl output should end with newline, got %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("jsonl output must be a single line plus trailing newline, got %q", out)
	}

	var decoded struct {
		Timestamp string `json:"timestamp"`
		Stream    string `json:"stream"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSuffix(out, "\n")), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if decoded.Timestamp != "2024-10-01T12:34:56.789Z" {
		t.Errorf("timestamp = %q", decoded.Timestamp)
	}
	if decoded.Stream != "my-stream" {
		t.Errorf("stream = %q", decoded.Stream)
	}
	if decoded.Message != "hello\n" {
		t.Errorf("message = %q (newline must round-trip)", decoded.Message)
	}
}

func TestFormatEvent_JSONL_EmptyMessage(t *testing.T) {
	// jsonl always emits the event: timestamp and stream are useful even when
	// the message is empty, and consumers (jq, etc.) decide what to filter.
	ts := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "", "my-stream", formatJSONL, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Timestamp string `json:"timestamp"`
		Stream    string `json:"stream"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSuffix(out, "\n")), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, out)
	}
	if decoded.Message != "" {
		t.Errorf("message = %q, want empty", decoded.Message)
	}
	if decoded.Timestamp == "" || decoded.Stream == "" {
		t.Errorf("timestamp/stream must still be populated: %+v", decoded)
	}
}

func TestFormatEvent_JSONL_EscapesSpecialChars(t *testing.T) {
	// Quotes, backslashes, embedded newlines, tabs, and HTML-ish chars must
	// all round-trip cleanly. SetEscapeHTML(false) means <, >, & stay literal.
	tricky := "with \"quotes\" and \\backslash and\nnewline\tand <html> & ampersand"
	out, err := formatEvent(0, tricky, "s", formatJSONL, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "\\u003c") || strings.Contains(out, "\\u003e") || strings.Contains(out, "\\u0026") {
		t.Errorf("HTML chars should not be \\u-escaped in jsonl output: %q", out)
	}

	var decoded struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSuffix(out, "\n")), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, out)
	}
	if decoded.Message != tricky {
		t.Errorf("message did not round-trip: got %q, want %q", decoded.Message, tricky)
	}
}

func TestFormatEvent_Raw_EmptyMessage(t *testing.T) {
	// formatEvent itself is pure: when called with an empty message it returns
	// "" verbatim. The CLI normally passes through ensureTrailingNewline first,
	// which would convert "" to "\n". Lock the function-level contract so that
	// future callers that bypass normalization see a stable result.
	out, err := formatEvent(0, "", "ignored-stream", formatRaw, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("got %q, want empty string", out)
	}
}

func TestFormatEvent_WithTime_LocalTimezone(t *testing.T) {
	// Exercise formatEvent in a non-UTC zone end-to-end (formatTimestamp is
	// already covered by TestFormatTimestamp, but the with-time path glues
	// timestamp + tab + message together and is worth pinning at this layer).
	jst := time.FixedZone("JST", 9*60*60)
	ts := time.Date(2024, 10, 1, 12, 34, 56, 789_000_000, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "hello\n", "s", formatWithTime, jst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2024-10-01T21:34:56.789+09:00\thello\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFormatEvent_WithTime_NoNormalize_PreservesBareCR(t *testing.T) {
	// When --no-normalize-newlines is in effect, the message reaches
	// formatEvent without normalization. A bare-CR-terminated message is
	// preserved as-is — formatEvent does NOT add a trailing \n in that case,
	// because doing so would silently undo the "raw bytes" mode the user
	// explicitly opted into. Downstream TSV consumers must not assume a
	// trailing \n in this combination.
	ts := time.Date(2024, 10, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	out, err := formatEvent(ts, "msg\r", "s", formatWithTime, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2024-10-01T12:00:00.000Z\tmsg\r\n"
	// formatEvent's HasSuffix("\n") check is false for "\r", so it appends \n.
	// Confirm the current behaviour explicitly so a future change is intentional.
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFormatEvent_JSONL_EmptyStream(t *testing.T) {
	// CloudWatch SDK returns nil pointers as empty strings via aws.ToString,
	// so a missing LogStreamName surfaces as "". Lock the JSON shape so
	// downstream `jq` filters that match `.stream` see an empty string rather
	// than a missing key or null.
	out, err := formatEvent(0, "msg\n", "", formatJSONL, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"stream":""`) {
		t.Errorf("expected empty stream field, got %q", out)
	}
}

func TestFormatEvent_JSONL_PreservesControlChars(t *testing.T) {
	// CloudWatch logs frequently contain ANSI escapes (ESC = \x1b) and even
	// NUL bytes from binary payloads. Verify that jsonl encoding round-trips
	// these through json.Unmarshal without loss.
	tricky := "ANSI \x1b[31mred\x1b[0m and NUL\x00here"
	out, err := formatEvent(0, tricky, "s", formatJSONL, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSuffix(out, "\n")), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %q", err, out)
	}
	if decoded.Message != tricky {
		t.Errorf("message did not round-trip: got %q, want %q", decoded.Message, tricky)
	}
}

func TestFormatEvent_UnknownFormat(t *testing.T) {
	_, err := formatEvent(0, "x", "s", "bogus", time.UTC)
	if err == nil {
		t.Errorf("expected error for unknown format, got nil")
	}
}
