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

import "io"
import "sync"

//var pool_dotBuffer = sync.Pool{ New : func() interface{} { return make([]byte,5) }}

/*
A io.Writer-Wrapper, that enables dot-line ended content to be written.
After the final ".\r\n", any further content is discarded. The final ".\r\n" is
addedd, if the content didn't contain any.
*/
type DotWriter struct{
	w io.Writer
	state uint16
	end bool
}
var pool_DotWriter = sync.Pool{ New : func() interface{} { return new(DotWriter) }}
func AcquireDotWriter() *DotWriter {
	return pool_DotWriter.Get().(*DotWriter)
}
func (w *DotWriter) Release() {
	// Remove the reference to it's writer to enable it to be GCed.
	w.w = nil
	pool_DotWriter.Put(w)
}
func (w *DotWriter) Reset(wr io.Writer) {
	*w = DotWriter{ w:wr, state : nlDotNl_start, end : false }
}
func (w *DotWriter) Write(buf []byte) (int, error) {
	// Do not write any further.
	if w.end { return len(buf),nil }
	
	// Put state into local variable. This enhances performance.
	state := w.state
	for i,b := range buf {
		state   = nlDotNl_transition(state,b)
		if state==nlDotNl_end {
			w.end = true
			j := i+1
			n,e := w.w.Write(buf[:j])
			if n<j {
				if e!=nil { e=io.EOF }
				return n,e
			}
			return len(buf),e
		}
	}
	// Put state back.
	w.state = state
	// Fully approved!
	return w.w.Write(buf)
}
var dotCRLF = []byte(".\r\n")
var crlfDotCRLF = []byte("\r\n.\r\n")
func (w *DotWriter) Close() (e error) {
	// short-cut:
	if w.end { return }
	
	if w.state == 0x0100 {
		_,e = w.Write(dotCRLF)
	}else{
		_,e = w.Write(crlfDotCRLF)
	}
	return e
}

