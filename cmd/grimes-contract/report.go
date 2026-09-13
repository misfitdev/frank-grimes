package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	anchor, err := anchorFromFlags(*path, *document, *section, *argument, uint32(*step), *source)
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
	runID := fs.String("run-id", "", "run identity; defaults to a generated one")
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
		report.RunId = runID2()
	}
	report.Target = &pb.Target{
		Root:              *root,
		Scope:             *scope,
		FingerprintSha256: targetFingerprint(*root, *scope),
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
		_, err := os.Stdout.Write(encoded)
		return err
	}
	fmt.Print(envelope.WrapReport(encoded))
	return nil
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
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return contracts.WriteAtomic(path, []byte(out))
}

func evidenceFromFlags(tier, claim string, anchor *pb.Anchor,
	action, cwd string, exitCode int, output, outputSum string,
	quote, assumption, reasoning, falsifier string,
) (*pb.Evidence, error) {
	e := &pb.Evidence{Claim: claim}
	switch strings.ToUpper(tier) {
	case "E1":
		e.Tier = pb.EvidenceTier_EVIDENCE_TIER_E1
		if action == "" {
			return nil, fmt.Errorf("E1 needs --action")
		}
		// int32 in the contract: a wider value would wrap and the evidence would
		// record an outcome the command did not have.
		if exitCode > math.MaxInt32 || exitCode < math.MinInt32 {
			return nil, fmt.Errorf("--exit-code must be between %d and %d",
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
			return nil, fmt.Errorf("E1 needs --output or --output-sha256")
		}
		e.Detail = &pb.Evidence_Reproduction{Reproduction: &pb.Reproduction{
			CompletedAt: timestamppb.New(contracts.Now().UTC()),
			Exhibit:     &pb.Reproduction_ExecutedCommand{ExecutedCommand: cmd},
		}}
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
