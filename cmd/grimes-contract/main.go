// grimes-contract validates, encodes, and transitions the Frank Grimes
// machine contract. It is the only supported way to read or write the ledger:
// hand-editing .grimes/ledger.pb bypasses every constraint in the schema.
package main

import (
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const usage = `grimes-contract - validator and codec for the Frank Grimes contract

Usage:
  grimes-contract id --category=SEC --path=<repo-path> [--evidence-file=<f>|--evidence=<s>] [--wide]
      Print the content-addressed fingerprint and stable finding ID.

  grimes-contract validate --type=<message> [--format=binary|textproto] [file]
      Validate a message. Binary input must also be in canonical encoding.

  grimes-contract encode-result [--format=textproto] [file]
      Read a GrimesResult, validate it, emit the canonical envelope.

  grimes-contract decode-result [file]
      Read an envelope or raw binary, validate, emit TextProto.

  grimes-contract ledger transition --ledger=<f> --id=<id> --to=<state>
      [--iteration=N] [--actor=<s>] [--evidence-sha256=<hex>]
      Apply one lifecycle transition and rewrite the ledger atomically.

  grimes-contract state --ledger=<f> [--json]
      Summarize ledger state.

Message types: Ledger, Finding, GrimesResult, LoopState, Verdict
States: open, fixed, verified, accepted, false_positive, regressed
`

const (
	envelopeBegin = "GRIMES_RESULT_PROTOBUF_V2_BEGIN"
	envelopeEnd   = "GRIMES_RESULT_PROTOBUF_V2_END"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "id":
		err = cmdID(os.Args[2:])
	case "validate":
		err = cmdValidate(os.Args[2:])
	case "encode-result":
		err = cmdEncodeResult(os.Args[2:])
	case "decode-result":
		err = cmdDecodeResult(os.Args[2:])
	case "ledger":
		err = cmdLedger(os.Args[2:])
	case "state":
		err = cmdState(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newMessage(name string) (proto.Message, error) {
	switch name {
	case "Ledger", "frank_grimes.v2.Ledger":
		return &pb.Ledger{}, nil
	case "Finding", "frank_grimes.v2.Finding":
		return &pb.Finding{}, nil
	case "GrimesResult", "frank_grimes.v2.GrimesResult":
		return &pb.GrimesResult{}, nil
	case "LoopState", "frank_grimes.v2.LoopState":
		return &pb.LoopState{}, nil
	case "Verdict", "frank_grimes.v2.Verdict":
		return &pb.Verdict{}, nil
	default:
		return nil, fmt.Errorf("unknown message type %q", name)
	}
}

func readInput(args []string) ([]byte, error) {
	if len(args) > 0 && args[0] != "-" {
		return os.ReadFile(args[0])
	}
	return io.ReadAll(os.Stdin)
}

func cmdID(args []string) error {
	fs := flag.NewFlagSet("id", flag.ExitOnError)
	category := fs.String("category", "", "canonical category code, e.g. SEC")
	path := fs.String("path", "", "repository-relative path")
	evidence := fs.String("evidence", "", "primary evidence text")
	evidenceFile := fs.String("evidence-file", "", "read evidence from a file")
	wide := fs.Bool("wide", false, "extend the ID to 16 hex after an observed collision")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *category == "" || *path == "" {
		return fmt.Errorf("--category and --path are required")
	}
	cat, err := contracts.ParseCategory(*category)
	if err != nil {
		return err
	}
	text := *evidence
	if *evidenceFile != "" {
		b, err := os.ReadFile(*evidenceFile)
		if err != nil {
			return err
		}
		text = string(b)
	}
	if text == "" {
		return fmt.Errorf("evidence is required: pass --evidence or --evidence-file")
	}
	fp := contracts.Fingerprint(cat, *path, text)
	fmt.Printf("fingerprint_sha256: %s\n", hex.EncodeToString(fp))
	fmt.Printf("id: %s\n", contracts.FindingID(cat, fp, *wide))
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	typeName := fs.String("type", "", "message type")
	format := fs.String("format", "binary", "binary or textproto")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *typeName == "" {
		return fmt.Errorf("--type is required")
	}
	m, err := newMessage(*typeName)
	if err != nil {
		return err
	}
	data, err := readInput(fs.Args())
	if err != nil {
		return err
	}
	switch *format {
	case "textproto":
		if err := contracts.ParseTextProto(data, m); err != nil {
			return err
		}
	case "binary":
		if err := contracts.UnmarshalCanonical(data, m); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
	fmt.Println("valid")
	return nil
}

func cmdEncodeResult(args []string) error {
	fs := flag.NewFlagSet("encode-result", flag.ExitOnError)
	format := fs.String("format", "textproto", "input format: textproto or binary")
	raw := fs.Bool("raw", false, "emit raw binary instead of the base64 envelope")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := readInput(fs.Args())
	if err != nil {
		return err
	}
	result := &pb.GrimesResult{}
	if *format == "textproto" {
		if err := contracts.ParseTextProto(data, result); err != nil {
			return err
		}
	} else if err := contracts.UnmarshalCanonical(data, result); err != nil {
		return err
	}
	encoded, err := contracts.EncodeCanonical(result)
	if err != nil {
		return err
	}
	if *raw {
		_, err := os.Stdout.Write(encoded)
		return err
	}
	fmt.Println(envelopeBegin)
	fmt.Println(base64.StdEncoding.EncodeToString(encoded))
	fmt.Println(envelopeEnd)
	return nil
}

// extractEnvelope takes the last complete envelope. An assistant message may
// contain earlier partial or quoted blocks; only the final complete one counts.
func extractEnvelope(s string) ([]byte, error) {
	begin := strings.LastIndex(s, envelopeBegin)
	if begin < 0 {
		return nil, fmt.Errorf("no %s marker found", envelopeBegin)
	}
	rest := s[begin+len(envelopeBegin):]
	end := strings.Index(rest, envelopeEnd)
	if end < 0 {
		return nil, fmt.Errorf("envelope opened but never closed")
	}
	payload := strings.Join(strings.Fields(rest[:end]), "")
	return base64.StdEncoding.DecodeString(payload)
}

func cmdDecodeResult(args []string) error {
	fs := flag.NewFlagSet("decode-result", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := readInput(fs.Args())
	if err != nil {
		return err
	}
	if strings.Contains(string(data), envelopeBegin) {
		data, err = extractEnvelope(string(data))
		if err != nil {
			return err
		}
	}
	result := &pb.GrimesResult{}
	if err := contracts.UnmarshalCanonical(data, result); err != nil {
		return err
	}
	out, err := prototextMarshal(result)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func cmdLedger(args []string) error {
	if len(args) == 0 || args[0] != "transition" {
		return fmt.Errorf("usage: grimes-contract ledger transition --ledger=<f> --id=<id> --to=<state>")
	}
	fs := flag.NewFlagSet("transition", flag.ExitOnError)
	ledgerPath := fs.String("ledger", contracts.LedgerPath, "ledger path")
	id := fs.String("id", "", "finding ID")
	to := fs.String("to", "", "target state")
	iteration := fs.Uint("iteration", 1, "iteration number")
	actor := fs.String("actor", "grimes", "who is making the change")
	evSum := fs.String("evidence-sha256", "", "hex evidence digest, required to reopen")
	ownerID := fs.String("owner", "", "human owner ID, required to accept")
	rationale := fs.String("rationale", "", "why the risk is accepted")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *id == "" || *to == "" {
		return fmt.Errorf("--id and --to are required")
	}
	stateVal, ok := pb.FindingStatus_value["FINDING_STATUS_"+strings.ToUpper(*to)]
	if !ok || stateVal == 0 {
		return fmt.Errorf("unknown state %q", *to)
	}

	data, err := os.ReadFile(*ledgerPath)
	if err != nil {
		return err
	}
	ledger := &pb.Ledger{}
	if err := contracts.UnmarshalCanonical(data, ledger); err != nil {
		dest, qerr := contracts.Quarantine(*ledgerPath, "invalid")
		if qerr == nil {
			return fmt.Errorf("ledger invalid, quarantined to %s: %w", dest, err)
		}
		return fmt.Errorf("ledger invalid: %w", err)
	}

	opts := contracts.TransitionOpts{
		Iteration: uint32(*iteration),
		Actor:     *actor,
	}
	if *evSum != "" {
		b, err := hex.DecodeString(*evSum)
		if err != nil {
			return fmt.Errorf("--evidence-sha256: %w", err)
		}
		opts.NewEvidenceSum = b
	}
	if *ownerID != "" {
		opts.Owner = &pb.HumanOwner{Id: *ownerID, Rationale: *rationale}
	}

	oscillation, err := contracts.Transition(ledger, *id, pb.FindingStatus(stateVal), opts)
	if err != nil {
		return err
	}
	encoded, err := contracts.EncodeCanonical(ledger)
	if err != nil {
		return err
	}
	if err := contracts.WriteAtomic(*ledgerPath, encoded); err != nil {
		return err
	}
	fmt.Printf("%s -> %s\n", *id, strings.ToLower(*to))
	if oscillation {
		fmt.Println("oscillation_detected: true")
	}
	return nil
}

func cmdState(args []string) error {
	fs := flag.NewFlagSet("state", flag.ExitOnError)
	ledgerPath := fs.String("ledger", contracts.LedgerPath, "ledger path")
	asJSON := fs.Bool("json", false, "emit the read-only JSON projection")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*ledgerPath)
	if err != nil {
		return err
	}
	ledger := &pb.Ledger{}
	if err := contracts.UnmarshalCanonical(data, ledger); err != nil {
		return err
	}
	if *asJSON {
		// A projection for reading. It is never accepted back as input.
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(ledger)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}
	counts := map[string]int{}
	for _, f := range ledger.GetFindings() {
		counts[strings.TrimPrefix(f.GetStatus().String(), "FINDING_STATUS_")]++
	}
	fmt.Printf("target: %s\n", ledger.GetTarget().GetScope())
	fmt.Printf("findings: %d\n", len(ledger.GetFindings()))
	for _, k := range []string{"OPEN", "FIXED", "VERIFIED", "ACCEPTED", "FALSE_POSITIVE", "REGRESSED"} {
		if counts[k] > 0 {
			fmt.Printf("  %s: %d\n", strings.ToLower(k), counts[k])
		}
	}
	return nil
}
