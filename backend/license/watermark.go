package license

// Watermark metadata yang disuntikkan secara dinamis saat diunduh dari LMS NgertiKode.
// Informasi ini adalah tanda tangan hak cipta dan jejak kepemilikan resmi pembeli.
var (
	WatermarkOwner       = "{{LICENSE_OWNER}}"
	WatermarkEmail       = "{{LICENSE_EMAIL}}"
	WatermarkOrderID     = "{{LICENSE_ORDER_ID}}"
	WatermarkFingerprint = "{{LICENSE_FINGERPRINT_HASH}}"
)

type WatermarkInfo struct {
	Owner       string `json:"owner"`
	Email       string `json:"email"`
	OrderID     string `json:"order_id"`
	Fingerprint string `json:"fingerprint"`
	IsBound     bool   `json:"is_bound"`
}

// GetWatermark mengembalikan informasi kepemilikan lisensi yang tertanam di source code.
func GetWatermark() WatermarkInfo {
	isBound := WatermarkOwner != "{{LICENSE_"+"OWNER}}" && WatermarkEmail != "{{LICENSE_"+"EMAIL}}"
	return WatermarkInfo{
		Owner:       WatermarkOwner,
		Email:       WatermarkEmail,
		OrderID:     WatermarkOrderID,
		Fingerprint: WatermarkFingerprint,
		IsBound:     isBound,
	}
}
