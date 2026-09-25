package platform

import "encoding/binary"

// findPIDInTCPTable answers "which process owns the connection from this local
// port to our proxy" from the table Windows' GetExtendedTcpTable returns
// (TCP_TABLE_OWNER_PID_CONNECTIONS, IPv4). No build tag, so its test runs
// anywhere. 0 means not found.
//
// The table is a count followed by rows of six little-endian 32-bit numbers:
//
//	state | local address | local port | remote address | remote port | pid
//
// Ports sit in the low 16 bits in network byte order (big-endian). Matching the
// local port AND a remote port of the proxy's picks the client's end; the
// proxy's own end of the same connection has them the other way round.
func findPIDInTCPTable(table []byte, localPort, proxyPort int) int {
	const rowSize = 24
	const established = 5

	if len(table) < 4 {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(table))
	rows := table[4:]
	port := func(v uint32) int { return int(v&0xff)<<8 | int(v>>8&0xff) }

	for i := 0; i < n && (i+1)*rowSize <= len(rows); i++ {
		row := rows[i*rowSize:]
		field := func(k int) uint32 { return binary.LittleEndian.Uint32(row[k*4:]) }
		if field(0) == established && port(field(2)) == localPort && port(field(4)) == proxyPort {
			return int(field(5))
		}
	}
	return 0
}
