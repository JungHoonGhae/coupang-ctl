package browser

import (
	"bytes"
	"encoding/base64"
	"image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// Firefox has no BarcodeDetector. Decode a bounded QR canvas/image in the
// owning process instead, without a native helper, network service, or file.
// The decoded value still must pass validQRLoginLink before presentation.
func decodeCamofoxQRImage(encoded string) (string, error) {
	if len(encoded) > 700000 {
		return "", ErrQRLinkUnavailable
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) > 512<<10 {
		return "", ErrQRLinkUnavailable
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 4<<20 {
		return "", ErrQRLinkUnavailable
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrQRLinkUnavailable
	}
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", ErrQRLinkUnavailable
	}
	result, err := qrcode.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true})
	if err != nil || len(result.GetText()) > 8192 {
		return "", ErrQRLinkUnavailable
	}
	return result.GetText(), nil
}
