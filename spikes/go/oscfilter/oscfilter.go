// Package oscfilter splits OSC sequences out of a PTY output stream before it
// reaches a VT emulator.
//
// Some emulators treat bytes 0x80-0x9F inside an OSC payload as 8-bit C1
// controls (0x9C = ST), so a UTF-8 window title such as "✳ Task" (E2 9C B3)
// ends the sequence early and the rest is printed on screen. This filter
// treats the payload as opaque bytes and only ends it on BEL or ESC \.
package oscfilter

// Filter is a streaming splitter; state survives across Write calls so a
// sequence split between PTY reads is handled.
type Filter struct {
	state   int // 0 ground, 1 saw ESC, 2 in OSC, 3 in OSC saw ESC
	payload []byte
	// OnOSC receives each complete OSC payload, e.g. "0;title" or "7;file://host/dir".
	OnOSC func(payload []byte)
}

const maxPayload = 64 * 1024

// Feed returns the bytes that should go to the emulator (everything except OSC).
func (f *Filter) Feed(p []byte) []byte {
	out := make([]byte, 0, len(p))
	for _, b := range p {
		switch f.state {
		case 0:
			if b == 0x1b {
				f.state = 1
				continue
			}
			out = append(out, b)
		case 1:
			if b == ']' {
				f.state = 2
				f.payload = f.payload[:0]
				continue
			}
			out = append(out, 0x1b, b)
			f.state = 0
			if b == 0x1b { // ESC ESC: keep the second as a new escape start
				out = out[:len(out)-1]
				f.state = 1
			}
		case 2:
			switch b {
			case 0x07: // BEL terminator
				f.emit()
			case 0x1b:
				f.state = 3
			default:
				if len(f.payload) < maxPayload {
					f.payload = append(f.payload, b)
				}
			}
		case 3:
			if b == '\\' { // ESC \ (ST)
				f.emit()
				continue
			}
			// Not a terminator: the ESC aborts the OSC and starts a new escape.
			f.state = 1
			f.payload = f.payload[:0]
			if b == ']' {
				f.state = 2
				continue
			}
			out = append(out, 0x1b, b)
			f.state = 0
		}
	}
	return out
}

func (f *Filter) emit() {
	if f.OnOSC != nil {
		f.OnOSC(append([]byte(nil), f.payload...))
	}
	f.payload = f.payload[:0]
	f.state = 0
}
