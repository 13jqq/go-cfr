package cfr

import (
	"math/rand"

	"github.com/timpalpant/go-cfr/internal/f32"
)

type RobustSamplingCFR struct {
	strategyProfile StrategyProfile
	k               int
	slicePool       *threadSafeFloatSlicePool
}

func NewRobustSampling(strategyProfile StrategyProfile, k int) *RobustSamplingCFR {
	return &RobustSamplingCFR{
		strategyProfile: strategyProfile,
		k:               k,
		slicePool:       &threadSafeFloatSlicePool{},
	}
}

func (c *RobustSamplingCFR) Run(node GameTreeNode) float32 {
	iter := c.strategyProfile.Iter()
	traversingPlayer := int(iter % 2)
	sampledActions := make(map[string]int)
	return c.runHelper(node, node.Player(), 1.0, traversingPlayer, sampledActions)
}

func (c *RobustSamplingCFR) runHelper(
	node GameTreeNode,
	lastPlayer int,
	sampleProb float32,
	traversingPlayer int,
	sampledActions map[string]int) float32 {

	var ev float32
	switch node.Type() {
	case TerminalNode:
		ev = float32(node.Utility(lastPlayer)) / sampleProb
	case ChanceNode:
		ev = c.handleChanceNode(node, lastPlayer, sampleProb, traversingPlayer, sampledActions)
	default:
		sgn := getSign(lastPlayer, node.Player())
		ev = sgn * c.handlePlayerNode(node, sampleProb, traversingPlayer, sampledActions)
	}

	node.Close()
	return ev
}

func (c *RobustSamplingCFR) handleChanceNode(node GameTreeNode, lastPlayer int, sampleProb float32, traversingPlayer int, sampledActions map[string]int) float32 {
	child, _ := node.SampleChild()
	// Sampling probabilities cancel out in the calculation of counterfactual value.
	return c.runHelper(child, lastPlayer, sampleProb, traversingPlayer, sampledActions)
}

func (c *RobustSamplingCFR) handlePlayerNode(node GameTreeNode, sampleProb float32, traversingPlayer int, sampledActions map[string]int) float32 {
	if traversingPlayer == node.Player() {
		return c.handleTraversingPlayerNode(node, sampleProb, traversingPlayer, sampledActions)
	} else {
		return c.handleSampledPlayerNode(node, sampleProb, traversingPlayer, sampledActions)
	}
}

func (c *RobustSamplingCFR) handleTraversingPlayerNode(node GameTreeNode, sampleProb float32, traversingPlayer int, sampledActions map[string]int) float32 {
	player := node.Player()
	nChildren := node.NumChildren()
	policy := c.strategyProfile.GetPolicy(node)
	strategy := policy.GetStrategy()

	// Sample min(k, |A|) actions with uniform probability.
	selected := arange(nChildren)
	if c.k < len(selected) {
		rand.Shuffle(len(selected), func(i, j int) {
			selected[i], selected[j] = selected[j], selected[i]
		})

		selected = selected[:c.k]
	}

	q := 1.0 / float32(nChildren)
	regrets := c.slicePool.alloc(nChildren)
	defer c.slicePool.free(regrets)
	var cfValue float32
	for _, i := range selected {
		child := node.GetChild(i)
		p := strategy[i]
		util := c.runHelper(child, player, q*sampleProb, traversingPlayer, sampledActions)
		regrets[i] = util
		cfValue += p * util
	}

	// Transform action utilities into instantaneous regrets by
	// subtracting out the expected utility over all possible actions.
	f32.AddConst(-cfValue, regrets)
	policy.AddRegret(1.0/q, regrets)
	return cfValue
}

func min(i, j int) int {
	if i < j {
		return i
	}

	return j
}

// Sample player action according to strategy, do not update policy.
// Save selected action so that they are reused if this infoset is hit again.
func (c *RobustSamplingCFR) handleSampledPlayerNode(node GameTreeNode, sampleProb float32, traversingPlayer int, sampledActions map[string]int) float32 {
	player := node.Player()
	key := node.InfoSet(player).Key()
	policy := c.strategyProfile.GetPolicy(node)

	i, ok := sampledActions[key]
	if !ok {
		// First time hitting this infoset during this run.
		// Sample according to current strategy profile.
		i = sampleOne(policy.GetStrategy())
		sampledActions[key] = i
	}

	// Update average strategy for this node.
	// We perform "stochastic" updates as described in the MC-CFR paper.
	policy.AddStrategyWeight(1.0 / sampleProb)

	child := node.GetChild(i)
	// Sampling probabilities cancel out in the calculation of counterfactual value,
	// so we don't include them here.
	return c.runHelper(child, player, sampleProb, traversingPlayer, sampledActions)
}

func arange(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i
	}
	return result
}