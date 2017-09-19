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

/*
A Writer for postings that consist of a Head and a Body.

Here is an example, of what kind of message will be split into Head and Body:

	Subject: This is the head
	Message-ID: Blubb blubb
	From: Hugo Meyer <h.meyer@example.org>
	 
	This is the 1. line of the Body.
	This is the 2. line of the Body.
	...
	This is the last line of the Body.
	. (trailing <CRLF>.<CRLF>)
*/
type HeadBodyWriter struct{
	head,body io.Writer
	state uint16
	end bool
}
var pool_HeadBodyWriter = sync.Pool{ New : func() interface{} { return new(HeadBodyWriter) }}
func AcquireHeadBodyWriter() *HeadBodyWriter {
	return pool_HeadBodyWriter.Get().(*HeadBodyWriter)
}
func (w *HeadBodyWriter) Release() {
	// Remove the references to it's writers to enable it to be GCed.
	w.head = nil
	w.body = nil
	pool_HeadBodyWriter.Put(w)
}
func (w *HeadBodyWriter) Reset(head,body io.Writer) {
	*w = HeadBodyWriter{ head:head, body:body, state: 0, end : false }
}

func (w *HeadBodyWriter) Write(buf []byte) (int, error) {
	// Write everything after the head to the body.
	if w.end { return w.body.Write(buf) }
	
	// Put state into local variable. This enhances performance.
	state := w.state
	for i,b := range buf {
		state   = nlNl_transition(state,b)
		if state==nlNl_end {
			w.end = true
			j := i+1
			n,e := w.head.Write(buf[:j])
			if n<j {
				if e!=nil { e=io.EOF }
				return n,e
			}
			m,e := w.body.Write(buf[j:])
			return j+m,e
		}
	}
	// Put state back.
	w.state = state
	// Fully approved!
	return w.head.Write(buf)
}


