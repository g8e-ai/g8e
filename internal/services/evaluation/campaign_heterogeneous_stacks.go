// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	HeterogeneousStackGenerationRule = "heterogeneous-v1"
)

// HeterogeneousStackHypothesis names one preregistered stack-selection strategy.
type HeterogeneousStackHypothesis string

const (
	HeterogeneousHypothesisAccuracy          HeterogeneousStackHypothesis = "accuracy"
	HeterogeneousHypothesisEfficiency        HeterogeneousStackHypothesis = "efficiency"
	HeterogeneousHypothesisSmallest          HeterogeneousStackHypothesis = "smallest"
	HeterogeneousHypothesisToolCalling       HeterogeneousStackHypothesis = "tool-calling"
	HeterogeneousHypothesisPrivacy           HeterogeneousStackHypothesis = "privacy"
	HeterogeneousHypothesisLocalThroughput   HeterogeneousStackHypothesis = "local-throughput"
	HeterogeneousHypothesisHomogeneousFamily HeterogeneousStackHypothesis = "homogeneous-family"
	HeterogeneousHypothesisLineageDiverse    HeterogeneousStackHypothesis = "lineage-diverse"
)

var preregisteredHeterogeneousHypotheses = []HeterogeneousStackHypothesis{
	HeterogeneousHypothesisAccuracy,
	HeterogeneousHypothesisEfficiency,
	HeterogeneousHypothesisSmallest,
	HeterogeneousHypothesisToolCalling,
	HeterogeneousHypothesisPrivacy,
	HeterogeneousHypothesisLocalThroughput,
	HeterogeneousHypothesisHomogeneousFamily,
	HeterogeneousHypothesisLineageDiverse,
}

// HeterogeneousCoverageMatrix records which variant-role placements are
// satisfied by the generated stack set.
type HeterogeneousCoverageMatrix struct {
	HypothesisStackCount int
	CoverageStackCount   int
	TotalStackCount      int
	VariantRoleCoverage  map[string][]string
}

// HeterogeneousStackSet is the frozen preregistered heterogeneous stack set for
// one campaign registry.
type HeterogeneousStackSet struct {
	CampaignID     string
	GenerationRule string
	Seed           uint64
	SetDigest      string
	VariantIDs     []string
	Stacks         []*evalv1.HeterogeneousStackDefinition
	Coverage       *HeterogeneousCoverageMatrix
}

// HeterogeneousStackGenerationRequest carries frozen inputs for deterministic
// stack generation.
type HeterogeneousStackGenerationRequest struct {
	CampaignID string
	Seed       uint64
	Variants   []*evalv1.ModelVariant
}

// GenerateHeterogeneousStackSet materializes the preregistered
// hypothesis stacks plus coverage stacks so every frozen variant appears in
// every role at least once.
func GenerateHeterogeneousStackSet(req HeterogeneousStackGenerationRequest) (*HeterogeneousStackSet, error) {
	if req.CampaignID == "" || len(req.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: generate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
	}
	variants := cloneSortedVariants(req.Variants)
	if len(variants) < 3 {
		return nil, fmt.Errorf("evaluation: generate heterogeneous stack set: at least three variants are required")
	}
	stacks := make([]*evalv1.HeterogeneousStackDefinition, 0, len(variants)*3)
	for _, hypothesis := range preregisteredHeterogeneousHypotheses {
		stack, err := buildHypothesisStack(req.CampaignID, hypothesis, variants, req.Seed)
		if err != nil {
			return nil, err
		}
		stacks = append(stacks, stack)
	}
	coverageStacks, err := buildCoverageStacks(req.CampaignID, variants, stacks, req.Seed)
	if err != nil {
		return nil, err
	}
	stacks = append(stacks, coverageStacks...)
	coverage := buildCoverageMatrix(stacks)
	variantIDs := make([]string, 0, len(variants))
	for _, variant := range variants {
		variantIDs = append(variantIDs, variant.GetVariantId())
	}
	set := &HeterogeneousStackSet{
		CampaignID:     req.CampaignID,
		GenerationRule: HeterogeneousStackGenerationRule,
		Seed:           req.Seed,
		VariantIDs:     variantIDs,
		Stacks:         stacks,
		Coverage:       coverage,
	}
	digest, err := ComputeHeterogeneousStackSetDigest(set)
	if err != nil {
		return nil, err
	}
	set.SetDigest = digest
	if err := ValidateHeterogeneousStackSet(set); err != nil {
		return nil, err
	}
	return set, nil
}

// ValidateHeterogeneousStackSet verifies stack digests, coverage, and set digest.
func ValidateHeterogeneousStackSet(set *HeterogeneousStackSet) error {
	if set == nil || set.CampaignID == "" || set.GenerationRule == "" || len(set.Stacks) == 0 || set.Coverage == nil {
		return fmt.Errorf("evaluation: validate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
	}
	expectedDigest, err := ComputeHeterogeneousStackSetDigest(set)
	if err != nil {
		return err
	}
	if set.SetDigest != expectedDigest {
		return fmt.Errorf("evaluation: validate heterogeneous stack set: set digest mismatch")
	}
	seenStackIDs := make(map[string]struct{}, len(set.Stacks))
	for _, stack := range set.Stacks {
		if stack == nil || stack.GetStackId() == "" {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
		}
		if _, exists := seenStackIDs[stack.GetStackId()]; exists {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: duplicate stack_id %q", stack.GetStackId())
		}
		seenStackIDs[stack.GetStackId()] = struct{}{}
		if err := ValidateHeterogeneousStackDigest(stack); err != nil {
			return err
		}
	}
	if set.GenerationRule == FormationCatalogStackGenerationRule {
		return validateFormationCatalogStackSet(set)
	}
	return validateVariantRoleCoverageComplete(set.VariantIDs, set.Stacks)
}

func validateVariantRoleCoverageComplete(variantIDs []string, stacks []*evalv1.HeterogeneousStackDefinition) error {
	coverage := roleCoverageFromStacks(stacks)
	for _, stack := range stacks {
		for _, slot := range []struct {
			role      evalv1.ModelCampaignRole
			variantID string
		}{
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY, stack.GetPrimarySlot().GetVariantId()},
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT, stack.GetAssistantSlot().GetVariantId()},
			{evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE, stack.GetLiteSlot().GetVariantId()},
		} {
			if slot.variantID == "" {
				return fmt.Errorf("evaluation: validate heterogeneous stack set: stack %s missing role binding", stack.GetStackId())
			}
		}
	}
	for _, variantID := range variantIDs {
		roles := coverage[variantID]
		if len(roles) != 3 {
			return fmt.Errorf("evaluation: validate heterogeneous stack set: variant %s missing role coverage (%d/3)", variantID, len(roles))
		}
	}
	return nil
}

// ComputeHeterogeneousMatrixSize returns stacks × scenarios.
func ComputeHeterogeneousMatrixSize(stackCount uint64) uint64 {
	return stackCount * StandardScenarioCount
}

func buildHypothesisStack(campaignID string, hypothesis HeterogeneousStackHypothesis, variants []*evalv1.ModelVariant, seed uint64) (*evalv1.HeterogeneousStackDefinition, error) {
	primary, assistant, lite, err := selectHypothesisVariants(hypothesis, variants, seed)
	if err != nil {
		return nil, err
	}
	return materializeStack(campaignID, "hypothesis-"+string(hypothesis), primary, assistant, lite)
}

func selectHypothesisVariants(hypothesis HeterogeneousStackHypothesis, variants []*evalv1.ModelVariant, seed uint64) (*evalv1.ModelVariant, *evalv1.ModelVariant, *evalv1.ModelVariant, error) {
	switch hypothesis {
	case HeterogeneousHypothesisAccuracy:
		ordered := sortedVariantsByParameterCount(variants, false)
		return pickDistinctVariants(ordered, 0, 1, 2)
	case HeterogeneousHypothesisEfficiency:
		ordered := sortedVariantsByParameterCount(variants, true)
		return pickMedianDistinctVariants(ordered)
	case HeterogeneousHypothesisSmallest:
		ordered := sortedVariantsByParameterCount(variants, true)
		return pickDistinctVariants(ordered, 0, 1, 2)
	case HeterogeneousHypothesisToolCalling:
		ordered := sortedVariantsByToolCalling(variants)
		return pickDistinctVariants(ordered, 0, 1, 2)
	case HeterogeneousHypothesisPrivacy:
		ordered := sortedVariantsByPrivacy(variants)
		return pickDistinctVariants(ordered, 0, 1, 2)
	case HeterogeneousHypothesisLocalThroughput:
		ordered := sortedVariantsByThroughput(variants, seed)
		return pickDistinctVariants(ordered, 0, 1, 2)
	case HeterogeneousHypothesisHomogeneousFamily:
		return pickHomogeneousFamilyVariants(variants)
	case HeterogeneousHypothesisLineageDiverse:
		return pickLineageDiverseVariants(variants)
	default:
		return nil, nil, nil, fmt.Errorf("evaluation: select hypothesis variants: unknown hypothesis %q", hypothesis)
	}
}

func buildCoverageStacks(campaignID string, variants []*evalv1.ModelVariant, existing []*evalv1.HeterogeneousStackDefinition, seed uint64) ([]*evalv1.HeterogeneousStackDefinition, error) {
	coverage := roleCoverageFromStacks(existing)
	anchors := defaultRoleAnchors(variants, seed)
	stacks := make([]*evalv1.HeterogeneousStackDefinition, 0)
	for {
		missing := missingVariantRolePairs(variants, coverage)
		if len(missing) == 0 {
			break
		}
		pair := missing[0]
		primary, assistant, lite := anchors.primary, anchors.assistant, anchors.lite
		switch pair.role {
		case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY:
			primary = pair.variant
		case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
			assistant = pair.variant
		case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
			lite = pair.variant
		}
		stackID := fmt.Sprintf("coverage-%s-%s", pair.variant.GetVariantId(), roleShortName(pair.role))
		stack, err := materializeStack(campaignID, stackID, primary, assistant, lite)
		if err != nil {
			return nil, err
		}
		stacks = append(stacks, stack)
		recordRoleCoverage(coverage, stack.GetPrimarySlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY))
		recordRoleCoverage(coverage, stack.GetAssistantSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT))
		recordRoleCoverage(coverage, stack.GetLiteSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE))
	}
	return stacks, nil
}

type variantRolePair struct {
	variant *evalv1.ModelVariant
	role    evalv1.ModelCampaignRole
}

type roleAnchors struct {
	primary   *evalv1.ModelVariant
	assistant *evalv1.ModelVariant
	lite      *evalv1.ModelVariant
}

func defaultRoleAnchors(variants []*evalv1.ModelVariant, seed uint64) roleAnchors {
	ordered := sortedVariantsByParameterCount(variants, true)
	primary, assistant, lite, err := pickMedianDistinctVariants(ordered)
	if err != nil {
		primary, assistant, lite, _ = pickDistinctVariants(ordered, 0, 1, 2)
	}
	return roleAnchors{primary: primary, assistant: assistant, lite: lite}
}

func missingVariantRolePairs(variants []*evalv1.ModelVariant, coverage map[string]map[string]struct{}) []variantRolePair {
	roles := []evalv1.ModelCampaignRole{
		evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
		evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT,
		evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE,
	}
	missing := make([]variantRolePair, 0)
	for _, variant := range variants {
		for _, role := range roles {
			roleName := roleShortName(role)
			if coverage[variant.GetVariantId()] == nil {
				missing = append(missing, variantRolePair{variant: variant, role: role})
				continue
			}
			if _, covered := coverage[variant.GetVariantId()][roleName]; !covered {
				missing = append(missing, variantRolePair{variant: variant, role: role})
			}
		}
	}
	sort.Slice(missing, func(i, j int) bool {
		left, right := missing[i], missing[j]
		if left.variant.GetVariantId() == right.variant.GetVariantId() {
			return left.role < right.role
		}
		return left.variant.GetVariantId() < right.variant.GetVariantId()
	})
	return missing
}

func roleCoverageFromStacks(stacks []*evalv1.HeterogeneousStackDefinition) map[string]map[string]struct{} {
	coverage := make(map[string]map[string]struct{})
	for _, stack := range stacks {
		recordRoleCoverage(coverage, stack.GetPrimarySlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY))
		recordRoleCoverage(coverage, stack.GetAssistantSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT))
		recordRoleCoverage(coverage, stack.GetLiteSlot().GetVariantId(), roleShortName(evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE))
	}
	return coverage
}

func buildCoverageMatrix(stacks []*evalv1.HeterogeneousStackDefinition) *HeterogeneousCoverageMatrix {
	coverage := roleCoverageFromStacks(stacks)
	public := make(map[string][]string, len(coverage))
	for variantID, roles := range coverage {
		roleList := make([]string, 0, len(roles))
		for role := range roles {
			roleList = append(roleList, role)
		}
		sort.Strings(roleList)
		public[variantID] = roleList
	}
	hypothesisCount := len(preregisteredHeterogeneousHypotheses)
	return &HeterogeneousCoverageMatrix{
		HypothesisStackCount: hypothesisCount,
		CoverageStackCount:   len(stacks) - hypothesisCount,
		TotalStackCount:      len(stacks),
		VariantRoleCoverage:  public,
	}
}

func recordRoleCoverage(coverage map[string]map[string]struct{}, variantID, role string) {
	if variantID == "" || role == "" {
		return
	}
	if coverage[variantID] == nil {
		coverage[variantID] = make(map[string]struct{})
	}
	coverage[variantID][role] = struct{}{}
}

func materializeStack(campaignID, stackID string, primary, assistant, lite *evalv1.ModelVariant) (*evalv1.HeterogeneousStackDefinition, error) {
	if primary == nil || assistant == nil || lite == nil {
		return nil, fmt.Errorf("evaluation: materialize heterogeneous stack: %w", constants.ErrMissingRequiredField)
	}
	if primary.GetVariantId() == assistant.GetVariantId() || primary.GetVariantId() == lite.GetVariantId() || assistant.GetVariantId() == lite.GetVariantId() {
		return nil, fmt.Errorf("evaluation: materialize heterogeneous stack %s: roles must bind distinct variants", stackID)
	}
	stack := &evalv1.HeterogeneousStackDefinition{
		StackId: stackID,
		PrimarySlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			VariantId:      primary.GetVariantId(),
		},
		AssistantSlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT,
			VariantId:      assistant.GetVariantId(),
		},
		LiteSlot: &evalv1.RoleAssignment{
			DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE,
			VariantId:      lite.GetVariantId(),
		},
	}
	digest, err := ComputeHeterogeneousStackDigest(stack)
	if err != nil {
		return nil, err
	}
	stack.StackDigest = digest
	return stack, nil
}

func cloneSortedVariants(variants []*evalv1.ModelVariant) []*evalv1.ModelVariant {
	cloned := make([]*evalv1.ModelVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		copyVariant, ok := proto.Clone(variant).(*evalv1.ModelVariant)
		if !ok {
			continue
		}
		cloned = append(cloned, copyVariant)
	}
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].GetVariantId() < cloned[j].GetVariantId()
	})
	return cloned
}

func sortedVariantsByParameterCount(variants []*evalv1.ModelVariant, ascending bool) []*evalv1.ModelVariant {
	ordered := cloneSortedVariants(variants)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i].GetParameterCount(), ordered[j].GetParameterCount()
		if left == right {
			return ordered[i].GetVariantId() < ordered[j].GetVariantId()
		}
		if ascending {
			return left < right
		}
		return left > right
	})
	return ordered
}

func sortedVariantsByToolCalling(variants []*evalv1.ModelVariant) []*evalv1.ModelVariant {
	ordered := cloneSortedVariants(variants)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank := toolCallingRank(ordered[i])
		rightRank := toolCallingRank(ordered[j])
		if leftRank == rightRank {
			if ordered[i].GetParameterCount() == ordered[j].GetParameterCount() {
				return ordered[i].GetVariantId() < ordered[j].GetVariantId()
			}
			return ordered[i].GetParameterCount() < ordered[j].GetParameterCount()
		}
		return leftRank < rightRank
	})
	return ordered
}

func toolCallingRank(variant *evalv1.ModelVariant) int {
	for _, observation := range variant.GetCapabilityObservations() {
		if observation.GetCapability() != evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING {
			continue
		}
		switch observation.GetOutcome() {
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
			return 0
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
			return 1
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED:
			return 2
		default:
			return 3
		}
	}
	return 4
}

func sortedVariantsByPrivacy(variants []*evalv1.ModelVariant) []*evalv1.ModelVariant {
	ordered := cloneSortedVariants(variants)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := quantizationRank(ordered[i].GetQuantization()), quantizationRank(ordered[j].GetQuantization())
		if left == right {
			if ordered[i].GetParameterCount() == ordered[j].GetParameterCount() {
				return ordered[i].GetVariantId() < ordered[j].GetVariantId()
			}
			return ordered[i].GetParameterCount() < ordered[j].GetParameterCount()
		}
		return left < right
	})
	return ordered
}

func quantizationRank(quantization string) int {
	switch strings.ToLower(strings.TrimSpace(quantization)) {
	case "q2", "q2_k", "q2_k_s":
		return 0
	case "q3", "q3_k", "q3_k_m", "q3_k_s":
		return 1
	case "q4", "q4_0", "q4_k", "q4_k_m", "q4_k_s":
		return 2
	case "q5", "q5_0", "q5_k", "q5_k_m", "q5_k_s":
		return 3
	case "q6", "q6_k", "q8", "q8_0":
		return 4
	case "f16", "fp16":
		return 5
	default:
		return 6
	}
}

func sortedVariantsByThroughput(variants []*evalv1.ModelVariant, seed uint64) []*evalv1.ModelVariant {
	ordered := sortedVariantsByParameterCount(variants, true)
	if len(ordered) <= 1 {
		return ordered
	}
	rotate := int(seed % uint64(len(ordered)))
	if rotate == 0 {
		return ordered
	}
	rotated := append([]*evalv1.ModelVariant(nil), ordered[rotate:]...)
	return append(rotated, ordered[:rotate]...)
}

func pickDistinctVariants(ordered []*evalv1.ModelVariant, primaryIndex, assistantIndex, liteIndex int) (*evalv1.ModelVariant, *evalv1.ModelVariant, *evalv1.ModelVariant, error) {
	if len(ordered) < 3 {
		return nil, nil, nil, fmt.Errorf("evaluation: pick distinct variants: at least three variants are required")
	}
	requested := []int{primaryIndex % len(ordered), assistantIndex % len(ordered), liteIndex % len(ordered)}
	resolved := make([]int, 3)
	used := make(map[int]struct{}, 3)
	for i, index := range requested {
		for offset := 0; offset < len(ordered); offset++ {
			candidate := (index + offset) % len(ordered)
			if _, exists := used[candidate]; exists {
				continue
			}
			resolved[i] = candidate
			used[candidate] = struct{}{}
			break
		}
	}
	if len(used) != 3 {
		return nil, nil, nil, fmt.Errorf("evaluation: pick distinct variants: unable to select three distinct variants")
	}
	return ordered[resolved[0]], ordered[resolved[1]], ordered[resolved[2]], nil
}

func pickMedianDistinctVariants(ordered []*evalv1.ModelVariant) (*evalv1.ModelVariant, *evalv1.ModelVariant, *evalv1.ModelVariant, error) {
	if len(ordered) < 3 {
		return nil, nil, nil, fmt.Errorf("evaluation: pick median distinct variants: at least three variants are required")
	}
	primary := ordered[len(ordered)/2]
	assistant := ordered[(len(ordered)/2+1)%len(ordered)]
	lite := ordered[(len(ordered)/2+2)%len(ordered)]
	if primary.GetVariantId() == assistant.GetVariantId() || primary.GetVariantId() == lite.GetVariantId() || assistant.GetVariantId() == lite.GetVariantId() {
		return pickDistinctVariants(ordered, len(ordered)/2, len(ordered)/2+1, len(ordered)/2+2)
	}
	return primary, assistant, lite, nil
}

func pickHomogeneousFamilyVariants(variants []*evalv1.ModelVariant) (*evalv1.ModelVariant, *evalv1.ModelVariant, *evalv1.ModelVariant, error) {
	familyCounts := make(map[string]int)
	for _, variant := range variants {
		family := strings.TrimSpace(variant.GetModelFamily())
		if family == "" {
			family = "unknown"
		}
		familyCounts[family]++
	}
	families := make([]string, 0, len(familyCounts))
	for family := range familyCounts {
		families = append(families, family)
	}
	sort.Slice(families, func(i, j int) bool {
		left, right := familyCounts[families[i]], familyCounts[families[j]]
		if left == right {
			return families[i] < families[j]
		}
		return left > right
	})
	targetFamily := families[0]
	familyVariants := make([]*evalv1.ModelVariant, 0)
	for _, variant := range variants {
		family := strings.TrimSpace(variant.GetModelFamily())
		if family == "" {
			family = "unknown"
		}
		if family == targetFamily {
			familyVariants = append(familyVariants, variant)
		}
	}
	if len(familyVariants) < 3 {
		return pickDistinctVariants(sortedVariantsByParameterCount(variants, false), 0, 1, 2)
	}
	return pickDistinctVariants(sortedVariantsByParameterCount(familyVariants, false), 0, 1, 2)
}

func pickLineageDiverseVariants(variants []*evalv1.ModelVariant) (*evalv1.ModelVariant, *evalv1.ModelVariant, *evalv1.ModelVariant, error) {
	familyLeaders := make(map[string]*evalv1.ModelVariant)
	for _, variant := range variants {
		family := strings.TrimSpace(variant.GetModelFamily())
		if family == "" {
			family = "unknown"
		}
		leader := familyLeaders[family]
		if leader == nil || variant.GetParameterCount() > leader.GetParameterCount() || (variant.GetParameterCount() == leader.GetParameterCount() && variant.GetVariantId() < leader.GetVariantId()) {
			familyLeaders[family] = variant
		}
	}
	leaders := make([]*evalv1.ModelVariant, 0, len(familyLeaders))
	for _, variant := range familyLeaders {
		leaders = append(leaders, variant)
	}
	sort.Slice(leaders, func(i, j int) bool {
		return leaders[i].GetVariantId() < leaders[j].GetVariantId()
	})
	if len(leaders) < 3 {
		return pickDistinctVariants(sortedVariantsByParameterCount(variants, false), 0, 1, 2)
	}
	return leaders[0], leaders[1], leaders[2], nil
}

func roleShortName(role evalv1.ModelCampaignRole) string {
	switch role {
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY:
		return "primary"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
		return "assistant"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
		return "lite"
	default:
		return role.String()
	}
}
