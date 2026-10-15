package screen

// oscFilter splits OSC sequences (ESC ] … BEL | ESC \) out of a PTY output
// stream before it reaches the emulator.
//
// charmbracelet/x/vt treats bytes 0x80–0x9F inside an OSC payload as 8-bit C1
// controls, so a UTF-8 title like "✳ Task" (E2 9C B3, where 0x9C is ST) ends
// the sequence early and the rest is printed on screen. Here the payload is
// opaque bytes and only BEL or ESC \ terminate it. The filter keeps state
// across calls, so a sequence split between PTY reads is handled.
type oscFilter struct {
	state   int // 0 ground, 1 saw ESC, 2 in OSC, 3 in OSC saw ESC
	payload []byte
	onOSC   func(payload []byte)
}

const maxOSCPayload = 64 * 1024

// feed appends to dst every byte that should reach the emulator and returns it.
func (f *oscFilter) feed(dst, p []byte) []byte {
	for _, b := range p {
		switch f.state {
		case 0:
			if b == 0x1b {
				f.state = 1
				continue
			}
			dst = append(dst, b)
		case 1:
			switch b {
			case ']':
				f.state = 2
				f.payload = f.payload[:0]
			case 0x1b: // ESC ESC: emit the first, keep waiting on the second
				dst = append(dst, 0x1b)
			default:
				dst = append(dst, 0x1b, b)
				f.state = 0
			}
		case 2:
			switch b {
			case 0x07:
				f.emit()
			case 0x1b:
				f.state = 3
			default:
				if len(f.payload) < maxOSCPayload {
					f.payload = append(f.payload, b)
				}
			}
		case 3:
			if b == '\\' {
				f.emit()
				continue
			}
			// ESC not followed by '\': the OSC is aborted and a new escape starts.
			f.payload = f.payload[:0]
			if b == ']' {
				f.state = 2
				continue
			}
			if b == 0x1b {
				f.state = 1
				continue
			}
			dst = append(dst, 0x1b, b)
			f.state = 0
		}
	}
	return dst
}

func (f *oscFilter) emit() {
	if f.onOSC != nil && len(f.payload) > 0 {
		f.onOSC(append([]byte(nil), f.payload...))
	}
	f.payload = f.payload[:0]
	f.state = 0
}
