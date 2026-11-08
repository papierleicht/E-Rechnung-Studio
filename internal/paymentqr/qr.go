// Package paymentqr encodes the byte-mode QR symbols used for EPC payments.
// It deliberately supports only error correction M and versions 1 through 13.
package paymentqr

import (
	"errors"
	"image"
	"image/color"
	"unicode/utf8"
)

// MaxPayloadBytes is the EPC payment payload limit and the capacity of a
// version 13, error correction M symbol in byte mode.
const MaxPayloadBytes = 331

// The format's codeword counts for versions 1–13, error correction level M.
// data is the total data capacity; parity is the parity count in each block.
var versions = [...]struct{ data, blocks, parity int }{
	{},
	{16, 1, 10}, {28, 1, 16}, {44, 1, 26}, {64, 2, 18},
	{86, 2, 24}, {108, 4, 16}, {124, 4, 18}, {154, 4, 22},
	{182, 5, 22}, {216, 5, 26}, {254, 5, 30}, {290, 8, 22},
	{334, 9, 22},
}

// Encode returns one black/white image pixel per QR module, without a quiet
// zone. Callers must add at least four white modules on each side when rendering
// it and use integer scaling. Payload bytes are preserved exactly. UTF-8 is
// declared by EPC's character-set field, so the QR does not insert an ECI segment.
func Encode(payload string) (image.Image, error) {
	if len(payload) > MaxPayloadBytes {
		return nil, errors.New("payment QR: payload exceeds 331 bytes")
	}
	if !utf8.ValidString(payload) {
		return nil, errors.New("payment QR: payload is not valid UTF-8")
	}
	version := 1
	for ; version < len(versions); version++ {
		countBits := 8
		if version >= 10 {
			countBits = 16
		}
		if 4+countBits+len(payload)*8 <= versions[version].data*8 {
			break
		}
	}
	words := codewords(payload, version)
	var best *symbol
	bestPenalty := int(^uint(0) >> 1)
	for mask := 0; mask < 8; mask++ {
		candidate := makeSymbol(words, version, mask)
		if penalty := candidate.penalty(); penalty < bestPenalty {
			best, bestPenalty = candidate, penalty
		}
	}
	return best, nil
}

type bitstream []bool

func (b *bitstream) append(value, count int) {
	for i := count - 1; i >= 0; i-- {
		*b = append(*b, value&(1<<i) != 0)
	}
}

// codewords constructs the byte segment, pads it, then splits and interleaves
// Reed–Solomon blocks. Longer blocks follow shorter blocks in the QR format.
func codewords(payload string, version int) []byte {
	info := versions[version]
	bits := make(bitstream, 0, info.data*8)
	bits.append(4, 4) // Byte mode.
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	bits.append(len(payload), countBits)
	for i := 0; i < len(payload); i++ {
		bits.append(int(payload[i]), 8)
	}
	bits.append(0, min(4, info.data*8-len(bits)))
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	data := make([]byte, len(bits)/8, info.data)
	for i, bit := range bits {
		if bit {
			data[i/8] |= 1 << (7 - i%8)
		}
	}
	for pad := byte(0xec); len(data) < info.data; pad ^= 0xec ^ 0x11 {
		data = append(data, pad)
	}
	blocks := make([][]byte, info.blocks)
	parity := make([][]byte, info.blocks)
	shortSize := info.data / info.blocks
	shortCount := info.blocks - info.data%info.blocks
	for i, offset := 0, 0; i < info.blocks; i++ {
		size := shortSize
		if i >= shortCount {
			size++
		}
		blocks[i] = data[offset : offset+size]
		parity[i] = errorCorrection(blocks[i], info.parity)
		offset += size
	}
	result := make([]byte, 0, info.data+info.blocks*info.parity)
	for column := 0; column <= shortSize; column++ {
		for _, block := range blocks {
			if column < len(block) {
				result = append(result, block[column])
			}
		}
	}
	for column := 0; column < info.parity; column++ {
		for _, block := range parity {
			result = append(result, block[column])
		}
	}
	return result
}

// multiply is multiplication in GF(256), with primitive polynomial 0x11d.
func multiply(a, b byte) byte {
	var product byte
	for b != 0 {
		if b&1 != 0 {
			product ^= a
		}
		high := a & 0x80
		a <<= 1
		if high != 0 {
			a ^= 0x1d
		}
		b >>= 1
	}
	return product
}

func errorCorrection(data []byte, degree int) []byte {
	// Coefficients are stored from highest to lowest power. The generator
	// has the consecutive roots 1, 2, 4, ... in GF(256).
	generator := []byte{1}
	root := byte(1)
	for i := 0; i < degree; i++ {
		next := make([]byte, len(generator)+1)
		for j, coefficient := range generator {
			next[j] ^= coefficient
			next[j+1] ^= multiply(coefficient, root)
		}
		generator = next
		root = multiply(root, 2)
	}
	remainder := make([]byte, len(data)+degree)
	copy(remainder, data)
	for i := range data {
		factor := remainder[i]
		for j, coefficient := range generator {
			remainder[i+j] ^= multiply(coefficient, factor)
		}
	}
	return remainder[len(data):]
}

type symbol struct {
	size     int
	modules  []bool
	reserved []bool
}

func (s *symbol) ColorModel() color.Model { return color.GrayModel }
func (s *symbol) Bounds() image.Rectangle { return image.Rect(0, 0, s.size, s.size) }
func (s *symbol) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= s.size || y >= s.size {
		return color.Gray{Y: 255}
	}
	if s.modules[y*s.size+x] {
		return color.Gray{Y: 0}
	}
	return color.Gray{Y: 255}
}

func (s *symbol) fixed(x, y int, dark bool) {
	if x >= 0 && y >= 0 && x < s.size && y < s.size {
		s.modules[y*s.size+x] = dark
		s.reserved[y*s.size+x] = true
	}
}

func makeSymbol(words []byte, version, mask int) *symbol {
	size := 17 + version*4
	s := &symbol{size, make([]bool, size*size), make([]bool, size*size)}
	for i := 8; i < size-8; i++ {
		s.fixed(i, 6, i%2 == 0)
		s.fixed(6, i, i%2 == 0)
	}
	for _, center := range [][2]int{{3, 3}, {size - 4, 3}, {3, size - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				radius := max(abs(dx), abs(dy))
				s.fixed(center[0]+dx, center[1]+dy, radius == 3 || radius <= 1)
			}
		}
	}
	var centers []int
	if version >= 2 && version <= 6 {
		centers = []int{6, size - 7}
	} else if version >= 7 {
		centers = []int{6, (size - 1) / 2, size - 7}
	}
	for row, cy := range centers {
		for column, cx := range centers {
			last := len(centers) - 1
			if (row == 0 && (column == 0 || column == last)) || (row == last && column == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					s.fixed(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	s.format(mask)
	if version >= 7 {
		value := protectedBits(version, 12, 0x1f25)
		for i := 0; i < 18; i++ {
			x, y := size-11+i%3, i/3
			dark := value&(1<<i) != 0
			s.fixed(x, y, dark)
			s.fixed(y, x, dark)
		}
	}
	bit := 0
	upward := true
	for right := size - 1; right > 0; right -= 2 {
		if right == 6 {
			right--
		}
		for step := 0; step < size; step++ {
			y := step
			if upward {
				y = size - 1 - step
			}
			for x := right; x >= right-1; x-- {
				index := y*size + x
				if s.reserved[index] {
					continue
				}
				dark := bit < len(words)*8 && words[bit/8]&(1<<(7-bit%8)) != 0
				s.modules[index] = dark != maskAt(mask, x, y)
				bit++
			}
		}
		upward = !upward
	}
	return s
}

// protectedBits appends a BCH remainder, for the format and version fields.
func protectedBits(data, degree, polynomial int) int {
	remainder := data << degree
	for bit := 31; bit >= degree; bit-- {
		if remainder&(1<<bit) != 0 {
			remainder ^= polynomial << (bit - degree)
		}
	}
	return data<<degree | remainder
}

func (s *symbol) format(mask int) {
	// Error correction M has format bits 00, followed by the three mask bits.
	value := protectedBits(mask, 10, 0x537) ^ 0x5412
	for i := 0; i < 15; i++ {
		dark := value&(1<<i) != 0
		switch {
		case i < 6:
			s.fixed(8, i, dark)
		case i == 6:
			s.fixed(8, 7, dark)
		case i == 7:
			s.fixed(8, 8, dark)
		case i == 8:
			s.fixed(7, 8, dark)
		default:
			s.fixed(14-i, 8, dark)
		}
		if i < 8 {
			s.fixed(s.size-1-i, 8, dark)
		} else {
			s.fixed(8, s.size-15+i, dark)
		}
	}
	s.fixed(8, s.size-8, true)
}

func maskAt(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (s *symbol) penalty() int {
	penalty, darkCount := 0, 0
	for y := 0; y < s.size; y++ {
		for x := 0; x < s.size; x++ {
			v := s.modules[y*s.size+x]
			if v {
				darkCount++
			}
			if x > 0 && y > 0 && v == s.modules[y*s.size+x-1] &&
				v == s.modules[(y-1)*s.size+x] && v == s.modules[(y-1)*s.size+x-1] {
				penalty += 3
			}
		}
	}
	for direction := 0; direction < 2; direction++ {
		for line := 0; line < s.size; line++ {
			previous, run, pattern := false, 0, 0
			// Four virtual white modules on each end account for the quiet zone
			// when penalizing finder-like patterns next to a symbol edge.
			for position := -4; position < s.size+4; position++ {
				dark := false
				if position >= 0 && position < s.size {
					index := line*s.size + position
					if direction == 1 {
						index = position*s.size + line
					}
					dark = s.modules[index]
					if position > 0 && dark == previous {
						run++
					} else {
						run = 1
					}
					if run == 5 {
						penalty += 3
					} else if run > 5 {
						penalty++
					}
					previous = dark
				}
				pattern = pattern << 1 & 0x7ff
				if dark {
					pattern |= 1
				}
				if position >= 6 && (pattern == 0x5d0 || pattern == 0x05d) {
					penalty += 40
				}
			}
		}
	}
	penalty += abs(darkCount*20-s.size*s.size*10) / (s.size * s.size) * 10
	return penalty
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
