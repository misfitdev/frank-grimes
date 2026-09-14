package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DefaultReportPath is where an in-progress report accumulates.
const DefaultReportPath = ".grimes/report.textproto"

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
          E3 --assumption --reasoning --falsifier`

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
	report.Candidates = append(report.Candidates, candidate)
	if err := saveReport(*file, report); err != nil {
		return err
	}
	fmt.Printf("added %s candidate at %s (%d total)\n",
		contracts.CategoryName(cat), anchorLabel(anchor), len(report.GetCandidates()))
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
	summary := fs.String("summary", "", "one-sentence BLUF")
	routed := fs.String("routed", "", "comma-separated categories routed this pass")
	examined := fs.Uint("examined", 0, "candidates examined during the self-grind")
	disproved := fs.Uint("disproved", 0, "candidates the self-grind disproved")
	raw := fs.Bool("raw", false, "write canonical bytes instead of the envelope")
	if err := fs.Parse(args); err != nil {
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
	report.Mode = pb.Mode_MODE_REPORT
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
	} else if _, err := fmt.Print(envelope.WrapReport(encoded)); err != nil {
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
	control, err := controlFromFlags(*mutation, *controlAction, *controlCwd, *controlExit, *controlOutput, *controlSum)
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
func controlFromFlags(mutation, action, cwd string, exitCode int, output, outputSum string) (*pb.NegativeControl, error) {
	offered := mutation != "" || action != "" || output != "" || outputSum != ""
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
	return &pb.NegativeControl{
		Mutation: mutation,
		Result:   result,
		// Derived, not asserted: a caller that could declare its own control
		// failed would be back to writing down the outcome it wanted.
		ProbeFailed: exitCode != 0,
	}, nil
}

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

// reproductionOf records a command that was actually run. The prefix names the
// flag family at fault, since an acquittal supplies two of them.
func reproductionOf(prefix, action, cwd string, exitCode int, output, outputSum string) (*pb.Reproduction, error) {
	if action == "" {
		return nil, fmt.Errorf("--%saction is required", prefix)
	}
	// int32 in the contract: a wider value would wrap and the record would
	// carry an outcome the command did not have.
	if exitCode > math.MaxInt32 || exitCode < math.MinInt32 {
		return nil, fmt.Errorf("--%sexit-code must be between %d and %d", prefix,
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
