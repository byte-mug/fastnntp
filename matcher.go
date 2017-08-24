/*
MIT License

Copyright (c) 2017 Simon Schmidt

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/


package fastnntp

/*
\r = 0x0d
\n = 0x0a
.  = 0x2e
-----------------------
(x,\r) -> x
(0,\n) -> 1
(1,. ) -> 2
(2,\n) -> 3
(_,_ ) -> 0
*/
const nlDotNl_start = 0x0100
const nlDotNl_end = 0x0300
func nlDotNl_transition(s uint16,b byte) uint16 {
	switch s|uint16(b) {
	// (x,\r) -> x
	case 0x010d,0x020d: return s
	// (0,\n) -> 1
	case 0x000a: return 0x0100
	// (1,. ) -> 2
	case 0x012e: return 0x0200
	// (2,\n) -> 3
	case 0x020a: return 0x0300
	}
	// (0,\r) -> 0
	// (_,_ ) -> 0
	return 0
}



/*
\r = 0x0d
\n = 0x0a
.  = 0x2e
-----------------------
(x,\r) -> x
(x,\n) -> x+1
(_,_ ) -> 0
*/
const nlNl_end = 0x0200
func nlNl_transition(s uint16,b byte) uint16 {
	switch b {
	case '\r': return s
	case '\n': return s+0x0100
	}
	return 0
}

func isWhiteSpace(i byte) bool {
	switch i {
	case ' ','\t': return true
	}
	return false
}

func endsWithLF(b []byte) bool {
	if len(b)==0 { return false }
	return b[len(b)-1]=='\n'
}
func trimCRLF(b []byte) []byte {
	i := len(b)
	for i>0 {
		i--
		switch b[i] {
		case '\r','\n':continue
		default: return b[:i+1]
		}
	}
	return b[:0]
}
func trimRight(buf []byte) []byte {
	for i,b := range buf {
		if b!=' ' { return buf[i:] }
	}
	return nil
}
func trimLeft(buf []byte) []byte {
	i := len(buf)
	for i>0 {
		i--
		switch buf[i] {
		case '\r','\n','\t',' ':continue
		default: return buf[:i+1]
		}
	}
	return nil
}

func toLower(b byte) byte {
	if (b >= 'A') && (b <= 'Z') { return (b-'A')+'a' }
	return b
}

