package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

func IsWAV(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE"
}

func DecodePCM16WAV(data []byte) ([]byte, int, error) {
	if !IsWAV(data) {
		return nil, 0, errors.New("audio is not a WAV file")
	}
	var channels uint16
	var bitsPerSample uint16
	var audioFormat uint16
	sampleRate := 0
	var pcm []byte

	for off := 12; off+8 <= len(data); {
		chunkID := string(data[off : off+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		chunkStart := off + 8
		chunkEnd := chunkStart + chunkSize
		if chunkSize < 0 || chunkEnd > len(data) {
			return nil, 0, errors.New("invalid WAV chunk size")
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, errors.New("invalid WAV fmt chunk")
			}
			audioFormat = binary.LittleEndian.Uint16(data[chunkStart : chunkStart+2])
			channels = binary.LittleEndian.Uint16(data[chunkStart+2 : chunkStart+4])
			sampleRate = int(binary.LittleEndian.Uint32(data[chunkStart+4 : chunkStart+8]))
			bitsPerSample = binary.LittleEndian.Uint16(data[chunkStart+14 : chunkStart+16])
		case "data":
			pcm = append([]byte(nil), data[chunkStart:chunkEnd]...)
		}
		off = chunkEnd
		if off%2 == 1 {
			off++
		}
	}
	if audioFormat != 1 {
		return nil, 0, fmt.Errorf("unsupported WAV encoding %d; only PCM is supported", audioFormat)
	}
	if bitsPerSample != 16 {
		return nil, 0, fmt.Errorf("unsupported WAV bit depth %d; only 16-bit PCM is supported", bitsPerSample)
	}
	if sampleRate <= 0 {
		return nil, 0, errors.New("missing WAV sample rate")
	}
	if len(pcm) == 0 {
		return nil, 0, errors.New("missing WAV data chunk")
	}
	if channels == 1 {
		if len(pcm)%2 != 0 {
			pcm = pcm[:len(pcm)-1]
		}
		return pcm, sampleRate, nil
	}
	if channels != 2 {
		return nil, 0, fmt.Errorf("unsupported WAV channel count %d", channels)
	}
	if len(pcm)%4 != 0 {
		pcm = pcm[:len(pcm)-(len(pcm)%4)]
	}
	mono := make([]byte, len(pcm)/2)
	for i, j := 0, 0; i+4 <= len(pcm); i, j = i+4, j+2 {
		l := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		r := int16(binary.LittleEndian.Uint16(pcm[i+2 : i+4]))
		m := int16((int(l) + int(r)) / 2)
		binary.LittleEndian.PutUint16(mono[j:j+2], uint16(m))
	}
	return mono, sampleRate, nil
}

func ResamplePCM16Mono(pcm []byte, srcRate, dstRate int) ([]byte, error) {
	if len(pcm)%2 != 0 {
		pcm = pcm[:len(pcm)-1]
	}
	if len(pcm) == 0 || srcRate <= 0 || dstRate <= 0 {
		return nil, errors.New("invalid pcm for resample")
	}
	if srcRate == dstRate {
		return append([]byte(nil), pcm...), nil
	}
	inSamples := len(pcm) / 2
	if inSamples <= 0 {
		return nil, errors.New("empty pcm")
	}
	outSamples := int(math.Round(float64(inSamples) * float64(dstRate) / float64(srcRate)))
	if outSamples <= 0 {
		return nil, errors.New("resample produced no samples")
	}
	out := make([]byte, outSamples*2)
	for i := 0; i < outSamples; i++ {
		pos := float64(i) * float64(srcRate) / float64(dstRate)
		idx := int(pos)
		if idx >= inSamples-1 {
			sample := int16(binary.LittleEndian.Uint16(pcm[(inSamples-1)*2:]))
			binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
			continue
		}
		frac := pos - float64(idx)
		s0 := float64(int16(binary.LittleEndian.Uint16(pcm[idx*2:])))
		s1 := float64(int16(binary.LittleEndian.Uint16(pcm[(idx+1)*2:])))
		sample := int16(math.Round(s0 + (s1-s0)*frac))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out, nil
}
