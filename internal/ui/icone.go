package ui

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

// Ícone da bandeja desenhado em tempo de execução (sem arquivo): disco verde
// Mesalvaí com um quadrado branco (papel). Cinza quando desconectado.
func icone(conectado bool) []byte {
	const n = 32
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	fundo := color.NRGBA{0x7a, 0xff, 0x00, 0xff}
	if !conectado {
		fundo = color.NRGBA{0x9a, 0x9a, 0x9a, 0xff}
	}
	c := float64(n-1) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			if dx*dx+dy*dy <= c*c {
				img.SetNRGBA(x, y, fundo)
			}
		}
	}
	for y := 10; y < 22; y++ {
		for x := 11; x < 21; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0xff, 0xff, 0xff, 0xff})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	if runtime.GOOS == "windows" {
		return pngParaIco(b.Bytes(), n)
	}
	return b.Bytes()
}

// ICO com uma imagem PNG dentro (Vista+ aceita).
func pngParaIco(p []byte, n int) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, []uint16{0, 1, 1})
	b.WriteByte(byte(n))
	b.WriteByte(byte(n))
	b.WriteByte(0)
	b.WriteByte(0)
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(32))
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(p)))
	_ = binary.Write(&b, binary.LittleEndian, uint32(22))
	b.Write(p)
	return b.Bytes()
}
