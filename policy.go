package cfr

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/timpalpant/go-cfr/internal/policy"
)

func init() {
	gob.Register(&PolicyTable{})
}

// PolicyTable implements traditional (tabular) CFR by storing accumulated
// regrets and strategy sums for each InfoSet, which is looked up by its Key().
type PolicyTable struct {
	params DiscountParams
	iter   int

	// Map of InfoSet Key -> policy for that infoset.
	policiesByKey map[string]*policy.Policy
	mayNeedUpdate map[*policy.Policy]struct{}
}

// NewPolicyTable creates a new PolicyTable with the given DiscountParams.
func NewPolicyTable(params DiscountParams) *PolicyTable {
	return &PolicyTable{
		params:        params,
		iter:          1,
		policiesByKey: make(map[string]*policy.Policy),
		mayNeedUpdate: make(map[*policy.Policy]struct{}),
	}
}

// Update performs regret matching for all nodes within this strategy profile that have
// been touched since the lapt call to Update().
func (pt *PolicyTable) Update() {
	discountPos, discountNeg, discountSum := pt.params.GetDiscountFactors(pt.iter)
	for np := range pt.mayNeedUpdate {
		np.NextStrategy(discountPos, discountNeg, discountSum)
	}

	pt.mayNeedUpdate = make(map[*policy.Policy]struct{})
	pt.iter++
}

func (pt *PolicyTable) Iter() int {
	return pt.iter
}

func (pt *PolicyTable) Close() error {
	return nil
}

func (pt *PolicyTable) GetPolicy(node GameTreeNode) NodePolicy {
	p := node.Player()
	is := node.InfoSet(p)
	key := is.Key()

	np, ok := pt.policiesByKey[key]
	if !ok {
		np = policy.New(node.NumChildren())
		pt.policiesByKey[key] = np
	}

	if np.NumActions() != node.NumChildren() {
		panic(fmt.Errorf("strategy has n_actions=%v but node has n_children=%v: %v",
			np.NumActions(), node.NumChildren(), node))
	}

	pt.mayNeedUpdate[np] = struct{}{}
	return np
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (pt *PolicyTable) UnmarshalBinary(buf []byte) error {
	r := bytes.NewReader(buf)
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&pt.params); err != nil {
		return err
	}

	if err := dec.Decode(&pt.iter); err != nil {
		return err
	}

	var nStrategies int64
	if err := dec.Decode(&nStrategies); err != nil {
		return err
	}

	pt.policiesByKey = make(map[string]*policy.Policy, nStrategies)
	for i := int64(0); i < nStrategies; i++ {
		var key string
		if err := dec.Decode(&key); err != nil {
			return err
		}

		var s policy.Policy
		if err := dec.Decode(&s); err != nil {
			return err
		}

		pt.policiesByKey[key] = &s
	}

	pt.mayNeedUpdate = make(map[*policy.Policy]struct{})
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (pt *PolicyTable) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pt.params); err != nil {
		return nil, err
	}

	if err := enc.Encode(pt.iter); err != nil {
		return nil, err
	}

	if err := enc.Encode(len(pt.policiesByKey)); err != nil {
		return nil, err
	}

	for key, p := range pt.policiesByKey {
		if err := enc.Encode(key); err != nil {
			return nil, err
		}

		if err := enc.Encode(p); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
