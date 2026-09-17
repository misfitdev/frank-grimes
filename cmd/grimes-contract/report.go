package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"github.com/misfitdev/frank-grimes/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultReportPath is where a report accumulates, inside the one directory a
// spawned role is allowed to write.
const DefaultReportPath = store.ReportPath

// cmdReport builds a provider report one candidate at a time.
//
// Authoring a constrained protobuf by hand is a poor thing to ask of a
// reviewer, so each candidate arrives as flags and is validated on the spot.
// Nothing here decides whether a finding is admissible: it builds the message
// and reports what the contract said, so the rules stay in one place.
func cmdReport(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", reportUsage)
	}
	switch args[0] {
	case "add":
		return cmdReportAdd(args[1:])
	case "seal":
		return cmdReportSeal(args[1:])
	case "units":
		return cmdReportUnits(args[1:])
	case "cover":
		return cmdReportCover(args[1:])
	case "stop":
		return cmdReportStop(args[1:])
	case "acquit":
		return cmdReportAcquit(args[1:])
	case "show":
		return cmdReportShow(args[1:])
	default:
		return fmt.Errorf("unknown report subcommand %q\n\n%s", args[0], reportUsage)
	}
}

const reportUsage = `report subcommands:
  report add   --category=<CODE> <anchor> --severity=<P0-P3> --likelihood=<L> --blast=<B> \
               --tier=<E1|E2|E3> --claim=<text> <evidence flags>
  report seal  [--run-id=<id>] --target-root=<path> --target-scope=<scope> \
               [--kind=code|document|idea|external] [--iteration=<n>] --summary=<text> \
               [--routed=COR,SEC,...] [--examined=<n>] [--disproved=<n>]
  report units [--inventory=<path>]
  report cover [--examined=<id,...>] [--examined-stdin0] \
               [--skip=<id> --skip-reason=<text> [--skip-material]]
  report stop  --category=<CODE> --condition=<marginal-yield|probes-exhausted|evidence-unavailable> \
               [--probes=<n>]
  report acquit --category=<CODE> <anchor> --claim=<text> --scope=<text> \
               --probe-action=<cmd> [--probe-cwd=<path>] [--probe-exit=<n>] --probe-output=<text> \
               [--control-mutation=<text> --control-action=<cmd> [--control-cwd=<path>] \
                --control-exit=<n> --control-output=<text>]
  report show

Anchor:   --path / --document with --section / --argument with --step / --source
Evidence: E1 --action --cwd --exit-code (--output | --output-sha256)
          E2 --quote
          E3 --assumption --reasoning --falsifier
Disproof: --disproof-action [--disproof-cwd] --disproof-exit \
            (--disproof-output | --disproof-output-sha256) [--disproof-contradicts]
          or --disproof-unavailable=<why>`

func cmdReportAdd(args []string) error {
	fs := flag.NewFlagSet("report add", flag.ExitOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	category := fs.String("category", "", "canonical category code, e.g. SEC")
	severity := fs.String("severity", "", "P0, P1, P2, or P3")
	likelihood := fs.String("likelihood", "likely", "likely, plausible, unlikely, or unknown")
	blast := fs.String("blast", "", "single-user, local-component, service, or systemic")
	tier := fs.String("tier", "", "E1, E2, or E3")
	claim := fs.String("claim", "", "what the evidence establishes")
	fix := fs.String("suggested-fix", "", "suggested fix, recorded as text in report mode")

	path := fs.String("path", "", "repository-relative path")
	document := fs.String("document", "", "supplied document name")
	section := fs.String("section", "", "section or clause within the document")
	argument := fs.String("argument", "", "supplied argument name")
	step := fs.Uint("step", 0, "numbered claim or step within the argument")
	source := fs.String("source", "", "retrieved source URI")
	publisher := fs.String("publisher", "", "who published the retrieved source")
	snapshot := fs.String("snapshot-sha256", "", "hex digest of the snapshot that was read")
	retrievedAt := fs.String("retrieved-at", "", "RFC 3339 time the source was read")
	sourceSection := fs.String("source-section", "", "section of the retrieved source")

	action := fs.String("action", "", "E1: the command or action performed")
	cwd := fs.String("cwd", ".", "E1: working directory the action ran in")
	exitCode := fs.Int("exit-code", 0, "E1: exit status of the action")
	output := fs.String("output", "", "E1: result excerpt demonstrating the behaviour")
	outputSum := fs.String("output-sha256", "", "E1: hex digest of the output instead of an excerpt")
	quote := fs.String("quote", "", "E2: exact quote in which the defect is visible")
	assumption := fs.String("assumption", "", "E3: the explicit assumption")
	reasoning := fs.String("reasoning", "", "E3: observed facts used by the inference")
	falsifier := fs.String("falsifier", "", "E3: an observation that would falsify it")

	disproofAction := fs.String("disproof-action", "", "the probe performed against this finding")
	disproofCwd := fs.String("disproof-cwd", ".", "working directory the disproof ran in")
	disproofExit := fs.Int("disproof-exit", 0, "exit status of the disproof")
	disproofOutput := fs.String("disproof-output", "", "result excerpt from the disproof")
	disproofSum := fs.String("disproof-output-sha256", "", "hex digest of the disproof output instead of an excerpt")
	disproofUnavailable := fs.String("disproof-unavailable", "", "why the disproof could not be attempted")
	contradicts := fs.Bool("disproof-contradicts", false, "the probe came back against the finding")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *category == "" || *severity == "" || *blast == "" || *tier == "" || *claim == "" {
		return fmt.Errorf("--category, --severity, --blast, --tier, and --claim are required")
	}
	if *step > math.MaxUint32 {
		return fmt.Errorf("--step must not exceed %d", uint32(math.MaxUint32))
	}

	cat, err := contracts.ParseCategory(strings.ToUpper(*category))
	if err != nil {
		return err
	}
	anchor, err := anchorFromFlags(*path, *document, *section, *argument, uint32(*step), *source, sourceMeta{
		publisher: *publisher, snapshot: *snapshot, retrieved: *retrievedAt,
		section: *sourceSection, required: true,
	})
	if err != nil {
		return err
	}
	sev, err := parseSeverity(*severity)
	if err != nil {
		return err
	}
	like, err := parseLikelihood(*likelihood)
	if err != nil {
		return err
	}
	radius, err := parseBlast(*blast)
	if err != nil {
		return err
	}
	evidence, err := evidenceFromFlags(*tier, *claim, anchor,
		*action, *cwd, *exitCode, *output, *outputSum,
		*quote, *assumption, *reasoning, *falsifier)
	if err != nil {
		return err
	}

	disproof, err := disproofFromFlags(fs, *disproofAction, *disproofCwd, *disproofExit,
		*disproofOutput, *disproofSum, *disproofUnavailable, *contradicts)
	if err != nil {
		return err
	}
	evidence.Disproof = disproof

	candidate := &pb.CandidateFinding{
		Category: cat,
		Location: &pb.Location{Anchor: anchor},
		Risk:     &pb.Risk{Severity: sev, Likelihood: like, BlastRadius: radius},
		Evidence: evidence,
	}
	if *fix != "" {
		candidate.SuggestedFix = fix
	}
	// Validated alone so the message names this candidate rather than failing
	// the whole report at seal time with no indication which one was at fault.
	if err := contracts.Validate(candidate); err != nil {
		return err
	}

	report, err := loadReport(*file)
	if err != nil {
		return err
	}
	if err := claimPass(*file, os.Getenv("GRIMES_PASS")); err != nil {
		return err
	}
	report.Candidates = append(report.Candidates, candidate)
	if err := saveReport(*file, report); err != nil {
		return err
	}
	fmt.Printf("added %s candidate at %s (%d total)\n",
		contracts.CategoryName(cat), anchorLabel(anchor), len(report.GetCandidates()))
	return nil
}

// deliverSealed writes the envelope where the engine will look for it.
//
// Every role seals through here. A report reaches the engine on the provider's
// stdout, which for an agent CLI means a model reproducing a base64 block as
// its last words; whether it does is a property of that CLI, and the engine
// holds no per-provider knowledge to predict it. Delivery becomes something
// this binary did instead.
//
// Only inside a pass: a report sealed outside one has no engine waiting on it,
// and a file named for no pass is one nothing would come back for.
//
// Written whole and renamed into place, since the engine collects it as soon as
// the pass returns and a partial file would read as a truncated report rather
// than as a write still in progress.
func deliverSealed(fallbackDir string, encoded []byte) (bool, error) {
	pass := os.Getenv("GRIMES_PASS")
	if pass == "" {
		return false, nil
	}
	// The engine names this directory absolutely, because a role is free to run
	// the contract CLI from wherever it likes and a relative path would then
	// deliver into somewhere nothing is waiting.
	dir := os.Getenv("GRIMES_WORK_DIR")
	if dir == "" {
		dir = fallbackDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("delivering the sealed report: %w", err)
	}
	final := filepath.Join(dir, filepath.Base(contracts.WorkEnvelopePath(pass)))
	tmp := final + ".partial"
	if err := os.WriteFile(tmp, []byte(envelope.WrapReport(encoded)), 0o600); err != nil {
		return false, fmt.Errorf("delivering the sealed report: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("delivering the sealed report: %w", err)
	}
	return true, nil
}

// emitSealed delivers the envelope and then writes it to stdout.
//
// Delivery comes first, and a stdout write that fails afterwards is not an
// error. The engine closes an over-limit pipe, and a role killed by its own
// broken stdout would take a report that had already been delivered with it:
// exiting cleanly is what lets that report be collected.
func emitSealed(fallbackDir string, encoded []byte) error {
	delivered, err := deliverSealed(fallbackDir, encoded)
	if err != nil {
		return err
	}
	if _, err := fmt.Print(envelope.WrapReport(encoded)); err != nil && !delivered {
		return err
	}
	return nil
}

func cmdReportSeal(args []string) error {
	fs := flag.NewFlagSet("report seal", flag.ExitOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	runID := fs.String("run-id", "", "run identity; defaults to $GRIMES_RUN_ID, then a generated one")
	targetFP := fs.String("target-fingerprint", "", "hex target fingerprint; defaults to $GRIMES_TARGET_FINGERPRINT")
	root := fs.String("target-root", "", "repository root, required for a code target")
	scope := fs.String("target-scope", "", "what was reviewed")
	kind := fs.String("kind", "code", "code, document, idea, or external")
	iteration := fs.Uint("iteration", 1, "iteration this report covers")
	mode := fs.String("mode", "report", "the mode the engine asked for; a report answers one request")
	summary := fs.String("summary", "", "one-sentence BLUF")
	routed := fs.String("routed", "", "comma-separated categories routed this pass")
	examined := fs.Uint("examined", 0, "candidates examined during the self-grind")
	disproved := fs.Uint("disproved", 0, "candidates the self-grind disproved")
	raw := fs.Bool("raw", false, "write canonical bytes instead of the envelope")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Before anything is built from it: a report another pass opened is not
	// this pass's to deliver.
	if err := sealablePass(*file, os.Getenv("GRIMES_PASS")); err != nil {
		return err
	}
	// The engine exports the request it is making when it invokes a provider, so
	// sealing inside that invocation needs no flags to restate it. A report
	// answers one request and these are what say which.
	if *root == "" {
		*root = os.Getenv("GRIMES_TARGET_ROOT")
	}
	if *scope == "" {
		*scope = os.Getenv("GRIMES_TARGET_SCOPE")
	}
	if !wasSetIn(fs, "kind") && os.Getenv("GRIMES_TARGET_KIND") != "" {
		*kind = os.Getenv("GRIMES_TARGET_KIND")
	}
	if !wasSetIn(fs, "iteration") && os.Getenv("GRIMES_ITERATION") != "" {
		n, err := strconv.ParseUint(os.Getenv("GRIMES_ITERATION"), 10, 32)
		if err != nil {
			return fmt.Errorf("GRIMES_ITERATION is not a number: %v", err)
		}
		*iteration = uint(n)
	}
	if *scope == "" || *summary == "" {
		return fmt.Errorf("--target-scope and --summary are required")
	}
	if *iteration == 0 || *iteration > math.MaxUint32 {
		return fmt.Errorf("--iteration must be between 1 and %d", uint32(math.MaxUint32))
	}
	// The contract carries these as uint32. A larger value would wrap into a
	// smaller one and the sealed report would record a count nobody supplied.
	for _, c := range []struct {
		flag string
		val  uint
	}{{"examined", *examined}, {"disproved", *disproved}} {
		if c.val > math.MaxUint32 {
			return fmt.Errorf("--%s must not exceed %d", c.flag, uint32(math.MaxUint32))
		}
	}

	report, err := loadReport(*file)
	if err != nil {
		return err
	}

	targetKind, err := parseTargetKind(*kind)
	if err != nil {
		return err
	}
	cats, err := parseCategories(*routed)
	if err != nil {
		return err
	}

	report.SchemaMajor = contracts.SchemaMajor
	report.RunId = *runID
	if report.RunId == "" {
		report.RunId = os.Getenv("GRIMES_RUN_ID")
	}
	if report.RunId == "" {
		report.RunId = runID2()
	}
	// The engine owns the target's identity: it is taken over the target's
	// content, which this cannot see. Echoing what the engine exported is what
	// binds the report to the run that asked for it; the root-and-scope digest
	// below is only for building a report outside a run.
	fpHex := *targetFP
	if fpHex == "" {
		fpHex = os.Getenv("GRIMES_TARGET_FINGERPRINT")
	}
	fp := targetFingerprint(*root, *scope)
	if fpHex != "" {
		decoded, err := hex.DecodeString(fpHex)
		if err != nil || len(decoded) != 32 {
			return fmt.Errorf("--target-fingerprint must be 64 hex characters")
		}
		fp = decoded
	}
	report.Target = &pb.Target{
		Root:              *root,
		Scope:             *scope,
		FingerprintSha256: fp,
		Display:           *scope,
		Kind:              targetKind,
	}
	switch strings.ToLower(*mode) {
	case "report":
		report.Mode = pb.Mode_MODE_REPORT
	case "fix":
		report.Mode = pb.Mode_MODE_FIX
	default:
		return fmt.Errorf("unknown mode %q", *mode)
	}
	report.Iteration = uint32(*iteration)
	report.RoutedCategories = cats
	report.CandidatesExamined = uint32(*examined)
	report.CandidatesDisproved = uint32(*disproved)
	report.Summary = *summary

	encoded, err := contracts.EncodeCanonical(report)
	if err != nil {
		return err
	}

	if *raw {
		if _, err := os.Stdout.Write(encoded); err != nil {
			return err
		}
	} else if err := emitSealed(filepath.Dir(*file), encoded); err != nil {
		return err
	}

	// Sealing ends this report, but only once it has been delivered. The
	// candidates in the working file are the only durable copy, and a write that
	// failed — an engine closing an over-limit pipe, say — would otherwise take
	// them with it, leaving nothing to inspect or retry.
	//
	// Clearing matters because the next seal stamps the current run's identity
	// onto whatever it finds: candidates left here would pass the binding and be
	// admitted again, re-reporting findings that pass never made.
	if err := os.Remove(*file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing the sealed report: %w", err)
	}
	if err := os.Remove(passPath(*file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing the sealed report: %w", err)
	}
	return nil
}

// cmdReportUnits lists what this review is accountable for, one unit id per
// line, exactly as the engine recorded them.
//
// A unit id is arbitrary text — a path, a heading, a URI — and prototext
// escapes it. Every provider grepping that rendering back into ids would get
// the fragile cases wrong in the same way, so the decoding happens once, here.
// Listing is not claiming: what to examine and what to skip stays the caller's
// statement.
func cmdReportUnits(args []string) error {
	fs := flag.NewFlagSet("report units", flag.ContinueOnError)
	path := fs.String("inventory", os.Getenv("GRIMES_TARGET_INVENTORY"), "inventory to read; defaults to $GRIMES_TARGET_INVENTORY")
	zero := fs.Bool("print0", false, "separate ids with NUL, for ids containing newlines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("report units needs --inventory or $GRIMES_TARGET_INVENTORY")
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	inventory := &pb.TargetInventory{}
	if err := contracts.UnmarshalCanonical(data, inventory); err != nil {
		return err
	}
	for _, u := range inventory.GetUnits() {
		if *zero {
			fmt.Printf("%s\x00", u.GetId())
			continue
		}
		fmt.Println(u.GetId())
	}
	return nil
}

// cmdReportCover accounts for the units of the target this review looked at.
//
// The denominator is the inventory the engine resolved, which this cannot see:
// the engine compares the two and refuses a unit that is not in the target. All
// this call can enforce is that the report stays self-consistent.
func cmdReportCover(args []string) error {
	fs := flag.NewFlagSet("report cover", flag.ContinueOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	examined := fs.String("examined", "", "comma-separated unit ids examined")
	stdin0 := fs.Bool("examined-stdin0", false, "read NUL-separated examined ids from stdin; the only form safe for an id containing a comma or a newline")
	skip := fs.String("skip", "", "unit id deliberately not examined; one per call")
	reason := fs.String("skip-reason", "", "why that unit was not examined")
	material := fs.Bool("skip-material", false, "the skipped unit could hold a defect that changes the verdict")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *skip == "" && (*reason != "" || *material) {
		return fmt.Errorf("--skip-reason and --skip-material need --skip")
	}
	// An unexplained skip is indistinguishable from an oversight.
	if *skip != "" && *reason == "" {
		return fmt.Errorf("--skip needs --skip-reason")
	}

	report, err := loadReport(*file)
	if err != nil {
		return err
	}
	if report.Coverage == nil {
		report.Coverage = &pb.UnitCoverage{}
	}
	for _, id := range strings.Split(*examined, ",") {
		if id = strings.TrimSpace(id); id != "" {
			report.Coverage.Examined = append(report.Coverage.Examined, id)
		}
	}
	if *stdin0 {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		for _, id := range strings.Split(string(raw), "\x00") {
			if id != "" {
				report.Coverage.Examined = append(report.Coverage.Examined, id)
			}
		}
	}
	if *skip != "" {
		report.Coverage.Skipped = append(report.Coverage.Skipped, &pb.SkippedUnit{
			UnitId:   *skip,
			Reason:   *reason,
			Material: *material,
		})
	}
	if err := contracts.Validate(report.GetCoverage()); err != nil {
		return err
	}
	if err := claimPass(*file, os.Getenv("GRIMES_PASS")); err != nil {
		return err
	}
	if err := saveReport(*file, report); err != nil {
		return err
	}
	fmt.Printf("covered %d examined, %d skipped\n",
		len(report.GetCoverage().GetExamined()), len(report.GetCoverage().GetSkipped()))
	return nil
}

// cmdReportStop records what ended a routed category's grind.
func cmdReportStop(args []string) error {
	fs := flag.NewFlagSet("report stop", flag.ContinueOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	category := fs.String("category", "", "routed category this stop is for")
	condition := fs.String("condition", "", "marginal-yield, probes-exhausted, or evidence-unavailable")
	probes := fs.Uint("probes", 0, "probes attempted in this category")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *probes > math.MaxUint32 {
		return fmt.Errorf("--probes must not exceed %d", uint32(math.MaxUint32))
	}
	cats, err := parseCategories(*category)
	if err != nil {
		return err
	}
	if len(cats) != 1 {
		return fmt.Errorf("--category takes exactly one category, got %q", *category)
	}
	cat := cats[0]
	cond, err := parseStopCondition(*condition)
	if err != nil {
		return err
	}

	report, err := loadReport(*file)
	if err != nil {
		return err
	}
	stop := &pb.CategoryStop{Category: cat, Condition: cond, ProbesAttempted: uint32(*probes)}
	if err := contracts.Validate(stop); err != nil {
		return err
	}
	report.CategoryStops = append(report.CategoryStops, stop)
	if err := claimPass(*file, os.Getenv("GRIMES_PASS")); err != nil {
		return err
	}
	if err := saveReport(*file, report); err != nil {
		return err
	}
	fmt.Printf("stopped %s as %s after %d probes\n", *category, *condition, *probes)
	return nil
}

// cmdReportAcquit records a claim this review attacked and could not break.
//
// The negative control is what separates an acquittal from a probe that was
// never capable of failing. It is optional here because a control that cannot
// be run is a real situation; an acquittal without one is simply worth nothing
// downstream rather than being refused.
func cmdReportAcquit(args []string) error {
	fs := flag.NewFlagSet("report acquit", flag.ContinueOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	category := fs.String("category", "", "canonical category code, e.g. SEC")
	claim := fs.String("claim", "", "the claim or invariant that was attacked")
	scope := fs.String("scope", "", "what this acquittal does and does not cover")

	path := fs.String("path", "", "repository-relative path")
	document := fs.String("document", "", "supplied document name")
	section := fs.String("section", "", "section or clause within the document")
	argument := fs.String("argument", "", "supplied argument name")
	step := fs.Uint("step", 0, "numbered claim or step within the argument")
	source := fs.String("source", "", "retrieved source URI")
	publisher := fs.String("publisher", "", "who published the retrieved source")
	snapshot := fs.String("snapshot-sha256", "", "hex digest of the snapshot that was read")
	retrievedAt := fs.String("retrieved-at", "", "RFC 3339 time the source was read")
	sourceSection := fs.String("source-section", "", "section of the retrieved source")

	probeAction := fs.String("probe-action", "", "the probe performed against the target as it is")
	probeCwd := fs.String("probe-cwd", ".", "working directory the probe ran in")
	probeExit := fs.Int("probe-exit", 0, "exit status of the probe")
	probeOutput := fs.String("probe-output", "", "result excerpt from the probe")
	probeSum := fs.String("probe-output-sha256", "", "hex digest of the probe output instead of an excerpt")

	mutation := fs.String("control-mutation", "", "what was changed or injected to make the defect present")
	controlAction := fs.String("control-action", "", "the probe performed against the mutated target")
	controlCwd := fs.String("control-cwd", ".", "working directory the control ran in")
	controlExit := fs.Int("control-exit", 0, "exit status of the probe against the mutated target")
	controlOutput := fs.String("control-output", "", "result excerpt from the control")
	controlSum := fs.String("control-output-sha256", "", "hex digest of the control output instead of an excerpt")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *category == "" || *claim == "" || *scope == "" {
		return fmt.Errorf("--category, --claim, and --scope are required")
	}
	if *step > math.MaxUint32 {
		return fmt.Errorf("--step must not exceed %d", uint32(math.MaxUint32))
	}

	cat, err := contracts.ParseCategory(strings.ToUpper(*category))
	if err != nil {
		return err
	}
	anchor, err := anchorFromFlags(*path, *document, *section, *argument, uint32(*step), *source, sourceMeta{
		publisher: *publisher, snapshot: *snapshot, retrieved: *retrievedAt,
		section: *sourceSection, required: true,
	})
	if err != nil {
		return err
	}
	probe, err := reproductionOf("probe-", *probeAction, *probeCwd, *probeExit, *probeOutput, *probeSum)
	if err != nil {
		return err
	}

	acquittal := &pb.Acquittal{
		Category:    cat,
		ClaimAnchor: anchor,
		Claim:       *claim,
		Probe:       probe,
		Scope:       *scope,
	}
	control, err := controlFromFlags(fs, *mutation, *controlAction, *controlCwd, *controlExit, *controlOutput, *controlSum)
	if err != nil {
		return err
	}
	acquittal.Control = control

	// Validated alone so the message names this acquittal rather than failing
	// the whole report at seal time with no indication which one was at fault.
	if err := contracts.Validate(acquittal); err != nil {
		return err
	}

	report, err := loadReport(*file)
	if err != nil {
		return err
	}
	report.Acquittals = append(report.Acquittals, acquittal)
	if err := claimPass(*file, os.Getenv("GRIMES_PASS")); err != nil {
		return err
	}
	if err := saveReport(*file, report); err != nil {
		return err
	}
	fmt.Printf("acquitted %s claim at %s (control %s)\n",
		contracts.CategoryName(cat), anchorLabel(anchor), controlLabel(control))
	return nil
}

// controlFromFlags builds the negative control, or returns nil when none was
// offered. A partial set is refused: a mutation nobody probed and a probe with
// nothing mutated are both records of something that did not happen.
//
// Offered is decided by which flags were passed rather than by their values,
// so --control-exit=0 alone is a partial control rather than no control. That
// case matters most: it is the one a provider reaches for when the control did
// not actually fail.
func controlFromFlags(fs *flag.FlagSet, mutation, action, cwd string, exitCode int, output, outputSum string) (*pb.NegativeControl, error) {
	offered := false
	fs.Visit(func(f *flag.Flag) {
		if strings.HasPrefix(f.Name, "control-") {
			offered = true
		}
	})
	if !offered {
		return nil, nil
	}
	if mutation == "" || action == "" {
		return nil, fmt.Errorf("a negative control needs --control-mutation and --control-action")
	}
	result, err := reproductionOf("control-", action, cwd, exitCode, output, outputSum)
	if err != nil {
		return nil, err
	}
	return &pb.NegativeControl{Mutation: mutation, Result: result}, nil
}

// controlLabel names what the record shows, which is read from the result
// rather than stated anywhere.
func controlLabel(c *pb.NegativeControl) string {
	if c == nil {
		return "unattempted"
	}
	return "failed as required"
}

func parseStopCondition(s string) (pb.StopCondition, error) {
	switch s {
	case "marginal-yield":
		return pb.StopCondition_STOP_CONDITION_MARGINAL_YIELD, nil
	case "probes-exhausted":
		return pb.StopCondition_STOP_CONDITION_PROBES_EXHAUSTED, nil
	case "evidence-unavailable":
		return pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE, nil
	default:
		return 0, fmt.Errorf("unknown stop condition %q", s)
	}
}

func cmdReportShow(args []string) error {
	fs := flag.NewFlagSet("report show", flag.ExitOnError)
	file := fs.String("file", DefaultReportPath, "report being built")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := loadReport(*file)
	if err != nil {
		return err
	}
	out, err := prototextMarshal(report)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// loadReport reads the in-progress report without validating it: it is
// incomplete by construction until seal fills in the run-level fields.
// passPath is where the pass that opened a report is recorded, beside it.
func passPath(reportPath string) string {
	return reportPath + ".pass"
}

// claimPass records which pass is accumulating this report, and refuses to add
// to one that another pass opened.
//
// A pass is a role the engine spawned, named by GRIMES_PASS. Outside one there
// is no pass to record: a review that builds a report before starting the run
// that will carry it is the documented sequence, and the run that seals it
// adopts what it finds. What this stops is the other case, where a pass died
// before sealing and the next pass delivers its candidates as its own.
func claimPass(reportPath, pass string) error {
	if pass == "" {
		return nil
	}
	// The report's own directory, which a role that builds its report in a
	// scratch directory of its own does not have yet. WriteAtomic makes it for
	// the report; the marker beside it needs the same.
	if err := os.MkdirAll(filepath.Dir(passPath(reportPath)), 0o755); err != nil {
		return err
	}
	// Exclusive create rather than read-then-write: two passes spawned into one
	// review directory would otherwise both find no marker, both write, and the
	// one whose write landed second would own a report holding the other's
	// candidates. Whoever creates the file owns the report.
	f, err := os.OpenFile(passPath(reportPath), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		return writePass(f, pass)
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	held, _, err := readPass(reportPath)
	if err != nil {
		return err
	}
	if held == pass {
		return nil
	}
	// An empty marker is one a pass created and did not live to write. It is
	// claimed by somebody; it is not claimed by this pass.
	return fmt.Errorf(
		"%s holds candidates another pass opened and did not seal; remove it or seal it there",
		reportPath)
}

// writePass records the pass in the marker it just created, reporting every
// way that can fail. A marker that was not written names nobody, and the caller
// is about to write candidates against it.
func writePass(f *os.File, pass string) error {
	if _, err := f.WriteString(pass); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readPass returns the pass that opened the report and whether a marker is
// there at all.
//
// The two are different answers. A marker that exists and is empty belongs to a
// pass that created it and has not written to it yet, which is a moment every
// claim passes through; reading that as no marker would let another pass take
// the report out from under it.
func readPass(reportPath string) (pass string, exists bool, err error) {
	data, err := os.ReadFile(passPath(reportPath))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(data)), true, nil
}

// sealablePass refuses to deliver candidates that belong to a pass other than
// this one.
//
// An unclaimed report is the documented sequence: a review accumulated it
// before the run existed, and this run carries it. A report claimed by another
// pass is the abandoned one, and stamping this run's identity onto it would
// deliver findings this pass never made.
func sealablePass(reportPath, pass string) error {
	held, exists, err := readPass(reportPath)
	if err != nil {
		return err
	}
	// No marker is the documented sequence; a marker naming this pass is its
	// own. An empty one is a claim in progress, and sealing here would take the
	// report and the marker with it.
	if !exists || (held != "" && held == pass) {
		return nil
	}
	return fmt.Errorf(
		"%s was opened by another pass and never sealed; its candidates are not this pass's to deliver",
		reportPath)
}

func loadReport(path string) (*pb.ProviderReport, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &pb.ProviderReport{}, nil
	}
	if err != nil {
		return nil, err
	}
	report := &pb.ProviderReport{}
	if err := prototextUnmarshal(data, report); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return report, nil
}

func saveReport(path string, report *pb.ProviderReport) error {
	out, err := prototextMarshal(report)
	if err != nil {
		return err
	}
	// Creating the directory here would hide it from WriteAtomic, which syncs
	// only the directories it creates itself; the new entry in the repository
	// would then never be made durable.
	return contracts.WriteAtomic(path, []byte(out))
}

// disproofFromFlags builds the record of what was done to disprove the finding,
// or returns nil when nothing was offered.
//
// An attempt and a reason it could not be attempted are exclusive: a caller
// with both has described two different reviews.
func disproofFromFlags(fs *flag.FlagSet, action, cwd string, exitCode int,
	output, outputSum, why string, contradicts bool,
) (*pb.DisproofAttempt, error) {
	var offered, performed []string
	fs.Visit(func(f *flag.Flag) {
		if !strings.HasPrefix(f.Name, "disproof-") {
			return
		}
		offered = append(offered, "--"+f.Name)
		if f.Name != "disproof-unavailable" {
			performed = append(performed, "--"+f.Name)
		}
	})
	if len(offered) == 0 {
		return nil, nil
	}
	// Presence, not value: --disproof-cwd carries a default, so a caller who
	// passed it would otherwise be indistinguishable from one who did not.
	// Dropping these silently would record an attempt nobody made as one
	// nobody could make, and the difference is the whole point of the record.
	if why != "" && len(performed) > 0 {
		return nil, fmt.Errorf("--disproof-unavailable describes an attempt that did not happen, so it cannot be given with %s",
			strings.Join(performed, ", "))
	}
	if why != "" {
		return &pb.DisproofAttempt{Outcome: &pb.DisproofAttempt_Unavailable{Unavailable: why}}, nil
	}
	result, err := reproductionOf("disproof-", action, cwd, exitCode, output, outputSum)
	if err != nil {
		return nil, err
	}
	return &pb.DisproofAttempt{
		Outcome:          &pb.DisproofAttempt_Performed{Performed: result},
		ContradictsClaim: contradicts,
	}, nil
}

// reproductionOf records a command that was actually run. The prefix names the
// flag family at fault, since an acquittal supplies two of them.
// exitFlag names the exit-status flag for a family. Unprefixed it is
// --exit-code; every prefixed form shortens it, so deriving one from the other
// by concatenation names a flag nobody defined.
func exitFlag(prefix string) string {
	if prefix == "" {
		return "exit-code"
	}
	return prefix + "exit"
}

func reproductionOf(prefix, action, cwd string, exitCode int, output, outputSum string) (*pb.Reproduction, error) {
	if action == "" {
		return nil, fmt.Errorf("--%saction is required", prefix)
	}
	// int32 in the contract: a wider value would wrap and the record would
	// carry an outcome the command did not have.
	if exitCode > math.MaxInt32 || exitCode < math.MinInt32 {
		return nil, fmt.Errorf("--%s must be between %d and %d", exitFlag(prefix),
			int32(math.MinInt32), int32(math.MaxInt32))
	}
	cmd := &pb.ExecutedCommand{
		Action:   action,
		Cwd:      &pb.RepoPath{Value: cwd},
		ExitCode: int32(exitCode),
	}
	switch {
	case output != "":
		cmd.Output = &pb.ExecutedCommand_OutputExcerpt{OutputExcerpt: output}
	case outputSum != "":
		sum, err := hexBytes(outputSum)
		if err != nil {
			return nil, err
		}
		cmd.Output = &pb.ExecutedCommand_OutputSha256{OutputSha256: sum}
	default:
		return nil, fmt.Errorf("--%soutput or --%soutput-sha256 is required", prefix, prefix)
	}
	return &pb.Reproduction{
		CompletedAt: timestamppb.New(contracts.Now().UTC()),
		Exhibit:     &pb.Reproduction_ExecutedCommand{ExecutedCommand: cmd},
	}, nil
}

func evidenceFromFlags(tier, claim string, anchor *pb.Anchor,
	action, cwd string, exitCode int, output, outputSum string,
	quote, assumption, reasoning, falsifier string,
) (*pb.Evidence, error) {
	e := &pb.Evidence{Claim: claim}
	switch strings.ToUpper(tier) {
	case "E1":
		e.Tier = pb.EvidenceTier_EVIDENCE_TIER_E1
		repro, err := reproductionOf("", action, cwd, exitCode, output, outputSum)
		if err != nil {
			return nil, err
		}
		e.Detail = &pb.Evidence_Reproduction{Reproduction: repro}
	case "E2":
		e.Tier = pb.EvidenceTier_EVIDENCE_TIER_E2
		if quote == "" {
			return nil, fmt.Errorf("E2 needs --quote")
		}
		e.Detail = &pb.Evidence_Citation{Citation: &pb.Citation{Anchor: anchor, Quote: quote}}
	case "E3":
		e.Tier = pb.EvidenceTier_EVIDENCE_TIER_E3
		if assumption == "" || reasoning == "" || falsifier == "" {
			return nil, fmt.Errorf("E3 needs --assumption, --reasoning, and --falsifier")
		}
		e.Detail = &pb.Evidence_Inference{Inference: &pb.Inference{
			Assumption: assumption, Reasoning: reasoning, Falsifier: falsifier,
		}}
	default:
		return nil, fmt.Errorf("unknown tier %q; use E1, E2, or E3", tier)
	}
	return e, nil
}
