package model

import (
	"fmt"
	"log/slog"
)

const (
	chunkWidth        int32 = 16
	sectionBlockCount       = 16 * 16 * 16
	minIndirectBits   uint8 = 4
	maxIndirectBits   uint8 = 8
	maxDirectBits     uint8 = 15
)

// BlockPosition is an absolute integer block coordinate.
type BlockPosition struct{ X, Y, Z int32 }

// ChunkColumn contains the block-state sections for one loaded chunk.
// Its storage is intentionally opaque; WorldViews exposes read-only queries.
type ChunkColumn struct {
	minY, height int32
	sections     []ChunkSection
	version      uint64
}

// ChunkSection retains the compact server palette representation.
type ChunkSection struct {
	nonAir uint16
	states blockStates
}

type blockStates interface {
	get(index int) uint32
	set(index int, value uint32) (blockStates, bool)
}

type singleValueStates struct{ value uint32 }

type indirectPaletteStates struct {
	palette []uint32
	packed  packedStates
}

type directPaletteStates struct{ packed packedStates }

type packedStates struct {
	bits  uint8
	words []uint64
}

func NewSingleValueSection(nonAir uint16, stateID uint32) (ChunkSection, error) {
	if nonAir > sectionBlockCount {
		return ChunkSection{}, fmt.Errorf("invalid non-air block count: %d", nonAir)
	}
	
	if (stateID == 0 && nonAir != 0) || (stateID != 0 && nonAir != sectionBlockCount) {
		return ChunkSection{}, fmt.Errorf("single-value section has inconsistent non-air block count: state=%d count=%d", stateID, nonAir)
	}
	
	return ChunkSection{
		nonAir: nonAir, 
		states: singleValueStates{value: stateID},
	}, nil
}

func NewIndirectPaletteSection(nonAir uint16, bits uint8, palette []uint32, words []uint64) (ChunkSection, error) {
	if nonAir > sectionBlockCount || bits < 1 || bits > maxIndirectBits || len(palette) == 0 || len(palette) > 1<<bits {
		return ChunkSection{}, fmt.Errorf("invalid indirect palette section")
	}
	
	packed, err := newPackedStates(bits, sectionBlockCount, words)
	if err != nil {
		return ChunkSection{}, err
	}
	
	for index := range sectionBlockCount {
		if int(packed.get(index)) >= len(palette) {
			return ChunkSection{}, fmt.Errorf("palette index out of range")
		}
	}
	
	if countPaletteNonAir(packed, palette) != nonAir {
		return ChunkSection{}, fmt.Errorf("indirect palette section has inconsistent non-air block count")
	}
	
	return ChunkSection{
		nonAir: nonAir, 
		states: indirectPaletteStates{
			palette: append([]uint32(nil), palette...), 
			packed: packed,
		}}, nil
}

func NewDirectPaletteSection(nonAir uint16, bits uint8, words []uint64) (ChunkSection, error) {
	if nonAir > sectionBlockCount || bits <= maxIndirectBits || bits > maxDirectBits {
		return ChunkSection{}, fmt.Errorf("invalid direct palette section bits: %d", bits)
	}
	
	packed, err := newPackedStates(bits, sectionBlockCount, words)
	if err != nil {
		return ChunkSection{}, err
	}
	
	if countDirectNonAir(packed) != nonAir {
		return ChunkSection{}, fmt.Errorf("direct palette section has inconsistent non-air block count")
	}
	
	return ChunkSection{
		nonAir: nonAir, 
		states: directPaletteStates{
			packed: packed,
		}}, nil
}

func NewChunkColumn(minY, height int32, sections []ChunkSection) (ChunkColumn, error) {
	if minY%chunkWidth != 0 || height <= 0 || height%chunkWidth != 0 || len(sections) != int(height/chunkWidth) {
		return ChunkColumn{}, fmt.Errorf("invalid chunk column geometry min_y=%d height=%d sections=%d", minY, height, len(sections))
	}
	
	for _, section := range sections {
		if section.states == nil {
			return ChunkColumn{}, fmt.Errorf("chunk section has no block states")
		}
	}
	
	return ChunkColumn{
		minY: minY, 
		height: height, 
		sections: append([]ChunkSection(nil), sections...),
	}, nil
}

func (c ChunkColumn) geometryMatches(dimension DimensionType) bool {
	return c.minY == dimension.MinY && c.height == dimension.Height
}

func (c *ChunkColumn) blockState(localX, y, localZ int32) (uint32, bool) {
	sectionIndex := (y - c.minY) >> 4
	if localX < 0 || localX >= chunkWidth || localZ < 0 || localZ >= chunkWidth || sectionIndex < 0 || int(sectionIndex) >= len(c.sections) {
		return 0, false
	}
	
	localY := y & 15
	index := int(localY<<8 | localZ<<4 | localX)
	return c.sections[sectionIndex].states.get(index), true
}

// StateAt reads a state from a column-local X/Z coordinate and absolute Y.
func (c ChunkColumn) StateAt(localX, y, localZ int32) (uint32, bool) {
	return c.blockState(localX, y, localZ)
}

func (c *ChunkColumn) setBlockState(localX, y, localZ int32, stateID uint32) bool {
	sectionIndex := (y - c.minY) >> 4
	if localX < 0 || localX >= chunkWidth || localZ < 0 || localZ >= chunkWidth || sectionIndex < 0 || int(sectionIndex) >= len(c.sections) {
		return false
	}
	
	localY := y & 15
	index := int(localY<<8 | localZ<<4 | localX)
	section := &c.sections[sectionIndex]
	oldState := section.states.get(index)
	states, ok := section.states.set(index, stateID)
	if !ok {
		return false
	}
	
	section.states = states
	if oldState == 0 && stateID != 0 {
		section.nonAir++
	} else if oldState != 0 && stateID == 0 {
		section.nonAir--
	}
	
	return true
}

func (s singleValueStates) get(index int) uint32 { return s.value }

func (s singleValueStates) set(index int, value uint32) (blockStates, bool) {
	if value == s.value {
		return s, true
	}
	packed, err := newPackedStates(minIndirectBits, sectionBlockCount, nil)
	if err != nil {
		return nil, false
	}
	packed.set(index, 1)
	return indirectPaletteStates{palette: []uint32{s.value, value}, packed: packed}, true
}

func (s indirectPaletteStates) get(index int) uint32 {
	return s.palette[s.packed.get(index)]
}

func (s indirectPaletteStates) set(index int, value uint32) (blockStates, bool) {
	paletteIndex := -1
	for i, candidate := range s.palette {
		if candidate == value {
			paletteIndex = i
			break
		}
	}
	if paletteIndex < 0 {
		paletteIndex = len(s.palette)
		if paletteIndex >= 1<<s.packed.bits {
			nextBits := s.packed.bits + 1
			if nextBits > maxIndirectBits {
				direct, ok := s.toDirect()
				if !ok {
					return nil, false
				}
				return direct.set(index, value)
			}
			s.packed = s.packed.resize(nextBits)
		}
		s.palette = append(s.palette, value)
	}
	s.packed.set(index, uint32(paletteIndex))
	return s, true
}

func (s indirectPaletteStates) toDirect() (directPaletteStates, bool) {
	packed, err := newPackedStates(maxDirectBits, sectionBlockCount, nil)
	if err != nil {
		return directPaletteStates{}, false
	}
	for index := range sectionBlockCount {
		packed.set(index, s.get(index))
	}
	return directPaletteStates{packed: packed}, true
}

func (s directPaletteStates) get(index int) uint32 { return s.packed.get(index) }

func (s directPaletteStates) set(index int, value uint32) (blockStates, bool) {
	if value > s.packed.mask() {
		return nil, false
	}
	s.packed.set(index, value)
	return s, true
}

func newPackedStates(bits uint8, capacity int, words []uint64) (packedStates, error) {
	if bits == 0 || bits > 32 {
		return packedStates{}, fmt.Errorf("invalid packed bits: %d", bits)
	}
	want := packedWordCount(capacity, bits)
	if words == nil {
		words = make([]uint64, want)
	}
	if len(words) != want {
		return packedStates{}, fmt.Errorf("invalid packed word count: got %d want %d", len(words), want)
	}
	return packedStates{bits: bits, words: append([]uint64(nil), words...)}, nil
}

func packedWordCount(capacity int, bits uint8) int {
	valuesPerWord := 64 / int(bits)
	return (capacity + valuesPerWord - 1) / valuesPerWord
}

func (p packedStates) get(index int) uint32 {
	valuesPerWord := 64 / int(p.bits)
	word := index / valuesPerWord
	shift := uint(index%valuesPerWord) * uint(p.bits)
	return uint32((p.words[word] >> shift) & uint64(p.mask()))
}

func (p *packedStates) set(index int, value uint32) {
	valuesPerWord := 64 / int(p.bits)
	word := index / valuesPerWord
	shift := uint(index%valuesPerWord) * uint(p.bits)
	mask := uint64(p.mask()) << shift
	p.words[word] = (p.words[word] &^ mask) | (uint64(value)&uint64(p.mask()))<<shift
}

func (p packedStates) resize(bits uint8) packedStates {
	next, _ := newPackedStates(bits, sectionBlockCount, nil)
	for index := range sectionBlockCount {
		next.set(index, p.get(index))
	}
	return next
}

func (p packedStates) mask() uint32 {
	return uint32((uint64(1) << p.bits) - 1)
}

func countPaletteNonAir(packed packedStates, palette []uint32) uint16 {
	var count uint16
	for index := range sectionBlockCount {
		if palette[packed.get(index)] != 0 {
			count++
		}
	}
	return count
}

func countDirectNonAir(packed packedStates) uint16 {
	var count uint16
	for index := range sectionBlockCount {
		if packed.get(index) != 0 {
			count++
		}
	}
	return count
}

func (s ChunkSection) NonAir() uint16 { return s.nonAir }

func (c ChunkColumn) Version() uint64  { return c.version }
func (c ChunkColumn) SectionCount() int { return len(c.sections) }
func (c ChunkColumn) NonAirBlocks() uint32 {
	var total uint32
	for _, s := range c.sections {
		total += uint32(s.nonAir)
	}
	return total
}
func (c ChunkColumn) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("version", c.version),
		slog.Int("sections", len(c.sections)),
		slog.Uint64("non_air", uint64(c.NonAirBlocks())),
	)
}
