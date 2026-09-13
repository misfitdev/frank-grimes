package envelope

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestWrapExtractRoundTrip(t *testing.T) {
	want := []byte{0x08, 0x02, 0x1a, 0x07, 'r', 'u', 'n', '-', '0', '0', '1'}
	got, err := Extract(Wrap(want))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestExtractIgnoresSurroundingProse(t *testing.T) {
	want := []byte("payload")
	s := "Here is my report.\n\n" + Wrap(want) + "\nThanks for reading.\n"
	got, err := Extract(s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractTakesLastCompleteEnvelope(t *testing.T) {
	first, second := []byte("first"), []byte("second")
	got, err := Extract(Wrap(first) + "revised:\n" + Wrap(second))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, second) {
		t.Errorf("got %q, want %q", got, second)
	}
}

func TestExtractSkipsEarlierTruncatedBlock(t *testing.T) {
	want := []byte("complete")
	s := Begin + "\ndGhpcyBnb3QgY3V0IG9mZg\n" + Wrap(want)
	got, err := Extract(s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A BEGIN emitted after a complete envelope, e.g. a stream cut mid-retry, must
// not shadow the good block that precedes it.
func TestExtractSkipsTrailingTruncatedBlock(t *testing.T) {
	want := []byte("complete")
	s := Wrap(want) + Begin + "\ndHJ1bmNhdGVk\n"
	got, err := Extract(s)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractRejectsMissingMarker(t *testing.T) {
	if _, err := Extract("no envelope here"); err == nil {
		t.Fatal("want error for absent marker")
	}
}

func TestExtractRejectsUnclosedEnvelope(t *testing.T) {
	_, err := Extract(Begin + "\ncGF5bG9hZA==\n")
	if err == nil {
		t.Fatal("want error for unclosed envelope")
	}
	if !strings.Contains(err.Error(), "never closed") {
		t.Errorf("got %q, want an unclosed-envelope error", err)
	}
}

func TestExtractRejectsNonBase64Payload(t *testing.T) {
	if _, err := Extract(Begin + "\n!!!not base64!!!\n" + End + "\n"); err == nil {
		t.Fatal("want error for undecodable payload")
	}
}

// Providers wrap long base64 across lines; folding must not corrupt the payload.
func TestExtractJoinsWrappedPayload(t *testing.T) {
	want := bytes.Repeat([]byte{0xab}, 96)
	encoded := base64.StdEncoding.EncodeToString(want)
	var folded strings.Builder
	for i := 0; i < len(encoded); i += 24 {
		end := min(i+24, len(encoded))
		folded.WriteString(encoded[i:end] + "\n")
	}
	got, err := Extract(Begin + "\n" + folded.String() + End + "\n")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestContains(t *testing.T) {
	if !Contains(Wrap([]byte("x"))) {
		t.Error("want Contains true for a wrapped payload")
	}
	if Contains("plain prose") {
		t.Error("want Contains false for plain prose")
	}
}

// The two marker pairs must not accept each other's payloads: a report sent
// where a result belongs should fail on the marker, not on validation.
func TestMarkersDoNotCrossAccept(t *testing.T) {
	payload := []byte("x")
	if _, err := Extract(WrapReport(payload)); err == nil {
		t.Error("a report envelope was accepted as a result")
	}
	if _, err := ExtractReport(Wrap(payload)); err == nil {
		t.Error("a result envelope was accepted as a report")
	}
	if Contains(WrapReport(payload)) {
		t.Error("Contains matched a report envelope")
	}
	if ContainsReport(Wrap(payload)) {
		t.Error("ContainsReport matched a result envelope")
	}
}

func TestReportRoundTrip(t *testing.T) {
	want := []byte{0x08, 0x02, 0x1a, 0x03, 'a', 'b', 'c'}
	got, err := ExtractReport("prose\n" + WrapReport(want) + "more prose\n")
	if err != nil {
		t.Fatalf("ExtractReport: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}
