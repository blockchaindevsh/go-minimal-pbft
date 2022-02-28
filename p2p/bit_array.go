package p2p

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
)

// BitArray is a thread-safe implementation of a bit array.
type BitArray struct {
	mtx   sync.Mutex
	Bits  int      `json:"bits"`  // NOTE: persisted via reflect, must be exported
	Elems []uint64 `json:"elems"` // NOTE: persisted via reflect, must be exported
}

// NewBitArray returns a new bit array.
// It returns nil if the number of bits is zero.
func NewBitArray(bits int) *BitArray {
	// // Reseed non-deterministically.
	// tmrand.Reseed()
	if bits <= 0 {
		return nil
	}
	bA := &BitArray{}
	bA.reset(bits)
	return bA
}

// reset changes size of BitArray to `bits` and re-allocates (zeroed) data buffer
func (bA *BitArray) reset(bits int) {
	bA.mtx.Lock()
	defer bA.mtx.Unlock()

	bA.Bits = bits
	if bits == 0 {
		bA.Elems = nil
	} else {
		bA.Elems = make([]uint64, numElems(bits))
	}
}

// Size returns the number of bits in the bitarray
func (bA *BitArray) Size() int {
	if bA == nil {
		return 0
	}
	return bA.Bits
}

// GetIndex returns the bit at index i within the bit array.
// The behavior is undefined if i >= bA.Bits
func (bA *BitArray) GetIndex(i int) bool {
	if bA == nil {
		return false
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()
	return bA.getIndex(i)
}

func (bA *BitArray) getIndex(i int) bool {
	if i >= bA.Bits {
		return false
	}
	return bA.Elems[i/64]&(uint64(1)<<uint(i%64)) > 0
}

// SetIndex sets the bit at index i within the bit array.
// The behavior is undefined if i >= bA.Bits
func (bA *BitArray) SetIndex(i int, v bool) bool {
	if bA == nil {
		return false
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()
	return bA.setIndex(i, v)
}

func (bA *BitArray) setIndex(i int, v bool) bool {
	if i >= bA.Bits {
		return false
	}
	if v {
		bA.Elems[i/64] |= (uint64(1) << uint(i%64))
	} else {
		bA.Elems[i/64] &= ^(uint64(1) << uint(i%64))
	}
	return true
}

// Copy returns a copy of the provided bit array.
func (bA *BitArray) Copy() *BitArray {
	if bA == nil {
		return nil
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()
	return bA.copy()
}

func (bA *BitArray) copy() *BitArray {
	c := make([]uint64, len(bA.Elems))
	copy(c, bA.Elems)
	return &BitArray{
		Bits:  bA.Bits,
		Elems: c,
	}
}

func (bA *BitArray) copyBits(bits int) *BitArray {
	c := make([]uint64, numElems(bits))
	copy(c, bA.Elems)
	return &BitArray{
		Bits:  bits,
		Elems: c,
	}
}

// IsEmpty returns true iff all bits in the bit array are 0
func (bA *BitArray) IsEmpty() bool {
	if bA == nil {
		return true // should this be opposite?
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()
	for _, e := range bA.Elems {
		if e > 0 {
			return false
		}
	}
	return true
}

// IsFull returns true iff all bits in the bit array are 1.
func (bA *BitArray) IsFull() bool {
	if bA == nil {
		return true
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()

	// Check all elements except the last
	for _, elem := range bA.Elems[:len(bA.Elems)-1] {
		if (^elem) != 0 {
			return false
		}
	}

	// Check that the last element has (lastElemBits) 1's
	lastElemBits := (bA.Bits+63)%64 + 1
	lastElem := bA.Elems[len(bA.Elems)-1]
	return (lastElem+1)&((uint64(1)<<uint(lastElemBits))-1) == 0
}

// PickRandom returns a random index for a set bit in the bit array.
// If there is no such value, it returns 0, false.
// It uses math/rand's global randomness Source to get this index.
func (bA *BitArray) PickRandom() (int, bool) {
	if bA == nil {
		return 0, false
	}

	bA.mtx.Lock()
	trueIndices := bA.getTrueIndices()
	bA.mtx.Unlock()

	if len(trueIndices) == 0 { // no bits set to true
		return 0, false
	}
	// nolint:gosec // G404: Use of weak random number generator
	// return trueIndices[mrand.Intn(len(trueIndices))], true
	// TODO
	return 0, true
}

func (bA *BitArray) getTrueIndices() []int {
	trueIndices := make([]int, 0, bA.Bits)
	curBit := 0
	numElems := len(bA.Elems)
	// set all true indices
	for i := 0; i < numElems-1; i++ {
		elem := bA.Elems[i]
		if elem == 0 {
			curBit += 64
			continue
		}
		for j := 0; j < 64; j++ {
			if (elem & (uint64(1) << uint64(j))) > 0 {
				trueIndices = append(trueIndices, curBit)
			}
			curBit++
		}
	}
	// handle last element
	lastElem := bA.Elems[numElems-1]
	numFinalBits := bA.Bits - curBit
	for i := 0; i < numFinalBits; i++ {
		if (lastElem & (uint64(1) << uint64(i))) > 0 {
			trueIndices = append(trueIndices, curBit)
		}
		curBit++
	}
	return trueIndices
}

// String returns a string representation of BitArray: BA{<bit-string>},
// where <bit-string> is a sequence of 'x' (1) and '_' (0).
// The <bit-string> includes spaces and newlines to help people.
// For a simple sequence of 'x' and '_' characters with no spaces or newlines,
// see the MarshalJSON() method.
// Example: "BA{_x_}" or "nil-BitArray" for nil.
func (bA *BitArray) String() string {
	return bA.StringIndented("")
}

// StringIndented returns the same thing as String(), but applies the indent
// at every 10th bit, and twice at every 50th bit.
func (bA *BitArray) StringIndented(indent string) string {
	if bA == nil {
		return "nil-BitArray"
	}
	bA.mtx.Lock()
	defer bA.mtx.Unlock()
	return bA.stringIndented(indent)
}

func (bA *BitArray) stringIndented(indent string) string {
	lines := []string{}
	bits := ""
	for i := 0; i < bA.Bits; i++ {
		if bA.getIndex(i) {
			bits += "x"
		} else {
			bits += "_"
		}
		if i%100 == 99 {
			lines = append(lines, bits)
			bits = ""
		}
		if i%10 == 9 {
			bits += indent
		}
		if i%50 == 49 {
			bits += indent
		}
	}
	if len(bits) > 0 {
		lines = append(lines, bits)
	}
	return fmt.Sprintf("BA{%v:%v}", bA.Bits, strings.Join(lines, indent))
}

// Bytes returns the byte representation of the bits within the bitarray.
func (bA *BitArray) Bytes() []byte {
	bA.mtx.Lock()
	defer bA.mtx.Unlock()

	numBytes := (bA.Bits + 7) / 8
	bytes := make([]byte, numBytes)
	for i := 0; i < len(bA.Elems); i++ {
		elemBytes := [8]byte{}
		binary.LittleEndian.PutUint64(elemBytes[:], bA.Elems[i])
		copy(bytes[i*8:], elemBytes[:])
	}
	return bytes
}

// Update sets the bA's bits to be that of the other bit array.
// The copying begins from the begin of both bit arrays.
func (bA *BitArray) Update(o *BitArray) {
	if bA == nil || o == nil {
		return
	}

	bA.mtx.Lock()
	o.mtx.Lock()
	copy(bA.Elems, o.Elems)
	o.mtx.Unlock()
	bA.mtx.Unlock()
}

func numElems(bits int) int {
	return (bits + 63) / 64
}
