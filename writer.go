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

import "fmt"
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



var pool_Overview = sync.Pool{ New : func() interface{} { return new(Overview) }}
type Overview struct{
	buffer []byte
	writer io.Writer
	mode   int
}
func (ov *Overview) reset(buffer []byte,writer io.Writer,mode int) {
	ov.buffer = buffer
	ov.writer = writer
	ov.mode   = mode
}
func (ov *Overview) release(){
	ov.writer = nil
	ov.buffer = nil
	ov.mode   = 0
	pool_Overview.Put(ov)
}
func (ov *Overview) WriteEntry(num int64,subject, from, date, msgId, refs []byte, lng, lines int64) error {
	out := ov.buffer
	switch ov.mode {
	case 0: // XOVER:
		out = append(AppendUint(out,num),'\t')
		out = append(append(out,subject...),'\t')
		out = append(append(out,from...),'\t')
		out = append(append(out,date...),'\t')
		out = append(append(out,msgId...),'\t')
		out = append(append(out,refs...),'\t')
		out = append(AppendUint(out,lng),'\t')
		out = append(AppendUint(out,lines),crlf...)
		
		// XHDR:
	case 1: out = append(append(out,subject...),crlf...)
	case 2: out = append(append(out,from...),crlf...)
	case 3: out = append(append(out,date...),crlf...)
	case 4: out = append(append(out,msgId...),crlf...)
	case 5: out = append(append(out,refs...),crlf...)
	case 6: out = append(AppendUint(out,lng),crlf...)
	case 7: out = append(AppendUint(out,lines),crlf...)
	default:
		panic(fmt.Sprint("invalid mode: ",ov.mode))
	}
	_,err := ov.writer.Write(out)
	return err
}

type IOverview interface{
	WriteEntry(num int64,subject, from, date, msgId, refs []byte, lng, lines int64) error
}

type ListActiveMode int
const(
	LAM_Full = ListActiveMode(iota)
	LAM_Active
	LAM_Newsgroups
)

var pool_ListActive = sync.Pool{ New : func() interface{} { return new(ListActive) }}
type ListActive struct{
	buffer []byte
	writer io.Writer
	mode   ListActiveMode
}
func (ov *ListActive) reset(buffer []byte,writer io.Writer,mode ListActiveMode) {
	ov.buffer = buffer
	ov.writer = writer
	ov.mode   = mode
}
func (ov *ListActive) release(){
	ov.writer = nil
	ov.buffer = nil
	ov.mode   = LAM_Active
	pool_ListActive.Put(ov)
}
func (ov *ListActive) GetListActiveMode() ListActiveMode { return ov.mode }
func (ov *ListActive) WriteActive(group []byte, high, low int64,status byte) error {
	if ov.mode != LAM_Active { panic(fmt.Sprint("invalid mode: ",ov.mode)) }
	out := ov.buffer
	out = append(out,group...)
	out = append(out,' ')
	out = AppendUint(out,high)
	out = append(out,' ')
	out = AppendUint(out,low)
	out = append(out,' ',status,'\r','\n')
	_,err := ov.writer.Write(out)
	return err
}
func (ov *ListActive) WriteNewsgroups(group []byte,description []byte) error {
	if ov.mode != LAM_Newsgroups { panic(fmt.Sprint("invalid mode: ",ov.mode)) }
	out := ov.buffer
	out = append(out,group...)
	out = append(out,'\t')
	out = append(out,description...)
	out = append(out,crlf...)
	_,err := ov.writer.Write(out)
	return err
}
func (ov *ListActive) WriteFullInfo(group []byte, high, low int64,status byte,description []byte) error {
	switch ov.mode {
	case LAM_Active:
		return ov.WriteActive(group,high,low,status)
	case LAM_Newsgroups:
		return ov.WriteNewsgroups(group,description)
	}
	panic(fmt.Sprint("invalid mode: ",ov.mode))
}

type IListActive interface{
	GetListActiveMode() ListActiveMode
	
	// This function may be used if, and only if GetListActiveMode() returns LAM_Active
	WriteActive(group []byte, high, low int64,status byte) error
	
	// This function may be used if, and only if GetListActiveMode() returns LAM_Newsgroups
	WriteNewsgroups(group []byte,description []byte) error
	
	WriteFullInfo(group []byte, high, low int64,status byte,description []byte) error
}

