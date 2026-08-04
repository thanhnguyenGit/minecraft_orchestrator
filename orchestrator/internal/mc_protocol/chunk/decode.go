// Package chunk decodes Minecraft's dimension-dependent chunkData payload.
package chunk

import (
	"bytes"
	"fmt"

	"minecraft_orchestrator/internal/engine/model"
	"minecraft_orchestrator/internal/mc_protocol/wire"
)

const (
	blockEntries = 16 * 16 * 16
	biomeEntries = 4 * 4 * 4
)

// DecodeColumn decodes the 1.21.5+ section stream carried by map_chunk.
func DecodeColumn(payload []byte, dimension model.DimensionType) (model.ChunkColumn, error) {
	if dimension.MinY%16 != 0 || dimension.Height <= 0 || dimension.Height%16 != 0 {
		return model.ChunkColumn{}, fmt.Errorf("invalid dimension geometry min_y=%d height=%d", dimension.MinY, dimension.Height)
	}

	r := bytes.NewReader(payload)
	sections := make([]model.ChunkSection, 0, dimension.Height/16)
	for sectionIndex := int32(0); sectionIndex < dimension.Height/16; sectionIndex++ {
		nonAir, err := wire.ReadInt16(r)
		if err != nil {
			return model.ChunkColumn{}, fmt.Errorf("read section %d block count: %w", sectionIndex, err)
		}

		if nonAir < 0 || nonAir > blockEntries {
			return model.ChunkColumn{}, fmt.Errorf("invalid section %d block count: %d", sectionIndex, nonAir)
		}

		section, err := readBlockStates(r, uint16(nonAir))
		if err != nil {
			return model.ChunkColumn{}, fmt.Errorf("read section %d block states: %w", sectionIndex, err)
		}

		if err := skipBiomeStates(r); err != nil {
			return model.ChunkColumn{}, fmt.Errorf("read section %d biome states: %w", sectionIndex, err)
		}

		sections = append(sections, section)
	}

	if err := wire.RequireEmpty(r); err != nil {
		return model.ChunkColumn{}, fmt.Errorf("chunk data: %w", err)
	}

	return model.NewChunkColumn(dimension.MinY, dimension.Height, sections)
}

func readBlockStates(r *bytes.Reader, nonAir uint16) (model.ChunkSection, error) {
	bits, err := r.ReadByte()
	if err != nil {
		return model.ChunkSection{}, err
	}

	if bits == 0 {
		stateID, err := readStateID(r)
		if err != nil {
			return model.ChunkSection{}, err
		}

		return model.NewSingleValueSection(nonAir, stateID)
	}

	if bits <= 8 {
		palette, err := readPalette(r, bits)
		if err != nil {
			return model.ChunkSection{}, err
		}

		words, err := readPackedWords(r, blockEntries, bits)
		if err != nil {
			return model.ChunkSection{}, err
		}

		return model.NewIndirectPaletteSection(nonAir, bits, palette, words)
	}
	if bits > 15 {
		return model.ChunkSection{}, fmt.Errorf("invalid direct block-state bits: %d", bits)
	}

	words, err := readPackedWords(r, blockEntries, bits)
	if err != nil {
		return model.ChunkSection{}, err
	}

	return model.NewDirectPaletteSection(nonAir, bits, words)
}

func skipBiomeStates(r *bytes.Reader) error {
	bits, err := r.ReadByte()
	if err != nil {
		return err
	}
	if bits == 0 {
		_, err := readStateID(r)
		if err != nil {
			return err
		}

		return nil
	}
	if bits > 8 {
		return fmt.Errorf("invalid biome bits: %d", bits)
	}
	if bits <= 3 {
		if _, err := readPalette(r, bits); err != nil {
			return err
		}
	}
	_, err = readPackedWords(r, biomeEntries, bits)
	return err
}

func readPalette(r *bytes.Reader, bits uint8) ([]uint32, error) {
	count, err := wire.ReadVarInt(r)
	if err != nil {
		return nil, err
	}

	if count <= 0 || count > 1<<bits {
		return nil, fmt.Errorf("invalid palette length: %d", count)
	}

	palette := make([]uint32, count)
	for index := range palette {
		if palette[index], err = readStateID(r); err != nil {
			return nil, err
		}
	}

	return palette, nil
}

func readPackedWords(r *bytes.Reader, entries int, bits uint8) ([]uint64, error) {
	if bits == 0 || bits > 32 {
		return nil, fmt.Errorf("invalid packed bits: %d", bits)
	}

	valuesPerWord := 64 / int(bits)
	wordCount := (entries + valuesPerWord - 1) / valuesPerWord

	words := make([]uint64, wordCount)
	for index := range words {
		word, err := wire.ReadInt64(r)
		if err != nil {
			return nil, err
		}
		words[index] = uint64(word)
	}

	return words, nil
}

func readStateID(r *bytes.Reader) (uint32, error) {
	stateID, err := wire.ReadVarInt(r)
	if err != nil {
		return 0, err
	}

	if stateID < 0 {
		return 0, fmt.Errorf("negative state ID: %d", stateID)
	}

	return uint32(stateID), nil
}
