package calibration

// CRC16Buypass is CRC-16/BUYPASS (also known as CRC-16/UMTS): polynomial
// 0x8005, initial value 0, no reflection, no final XOR.
func CRC16Buypass(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
