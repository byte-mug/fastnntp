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

type buffer struct{
	b []byte
	pos int
	limit int
}
func (b *buffer) read() []byte { return b.b[b.pos:b.limit] }
func (b *buffer) write() []byte { return b.b[b.limit:] }
func (b *buffer) advanceRead(i int) *buffer { b.pos+=i ; return b }
func (b *buffer) advanceWrite(i int) *buffer { b.limit+=i ; return b }
func (b *buffer) reset() *buffer {
	b.pos = 0
	b.limit = 0
	return b
}
func (b *buffer) feedFrom(r io.Reader) error {
	i,e := r.Read(b.write())
	if i>0 { b.advanceWrite(i) }
	return e
}
/*
var pool_buffer = sync.Pool{ New : func() interface{} { return &buffer{
	b : make([]byte,1<<13),
}} }
func (b *buffer) release() { pool_buffer.Put(b) }
*/


/*
A reader.
*/
type Reader struct {
	b *buffer
	r io.Reader
	e error
}
var pool_Reader = sync.Pool{ New : func() interface{} {
	return &Reader{ b: &buffer{b: make([]byte,1<<13)}}
}}
func AcquireReader() *Reader {
	return pool_Reader.Get().(*Reader)
}
func (r *Reader) Release() {
	pool_Reader.Put(r)
}
func (r *Reader) Init(rdr io.Reader) *Reader{
	r.b.reset()
	r.r = rdr
	r.e = nil
	return r
}

func (r *Reader) Read(b []byte) (int,error) {
	d := r.b.read()
	if len(d) > len(b) {
		copy(b,d)
		r.b.advanceRead(len(b))
		return len(b),nil
	}
	if len(d) > 0 {
		copy(b,d)
		r.b.reset()
		return len(d),nil
	}
	if r.e!=nil { return 0,r.e }
	return r.r.Read(b)
}
func (r *Reader) ReadLineB(ext []byte) ([]byte,error) {
	for {
		buf := r.b.read()
		for i,b := range buf {
			if b=='\n' {
				ext = append(ext,buf[:i+1]...)
				r.b.advanceRead(i+1)
				return ext,nil
			}
		}
		if len(buf) > 0 {
			ext = append(ext,buf...)
		}
		r.b.reset()
		e := r.b.feedFrom(r.r)
		if e!=nil { return ext,e }
	}
	panic("unreachable")
}

func (r *Reader) isContinuation() (ok bool,err error) {
	buf := r.b.read()
	if len(buf) == 0 {
		r.b.reset()
		err = r.b.feedFrom(r.r)
	}
	if len(buf) == 0 {
		return
	}
	if !isWhiteSpace(buf[0]) { return }
	ok = true
	
	// consume all leading white spaces
	for{
		for i,b := range buf {
			if !isWhiteSpace(b) {
				if i>0 { r.b.advanceRead(i) }
				return
			}
		}
		r.b.reset()
		err = r.b.feedFrom(r.r)
		buf = r.b.read()
		if len(buf) == 0 { return }
	}
	return
}

// Don't use it.
func (r *Reader) ReadContinuedLineB(ext []byte) ([]byte,error) {
	ext,err := r.ReadLineB(ext)
	if !endsWithLF(ext) { return ext,err }
	cont,err := r.isContinuation()
	for cont {
		ext = append(trimCRLF(ext),' ')
		ext,err = r.ReadLineB(trimCRLF(ext))
		cont,err = r.isContinuation()
	}
	return ext,err
}

type DotReader struct{
	r     *Reader
	state uint16
	data  []byte
	end   bool
	err   error
}
func (d *DotReader) innerRead() {
	if d.end || len(d.data) > 0 { return }
	buf := d.r.b.read()
	if len(buf) == 0 {
		d.r.b.reset()
		e := d.r.b.feedFrom(d.r.r)
		buf = d.r.b.read()
		if e!=nil { d.err = e }
	}
	// Put state into local variable. This enhances performance.
	state := d.state
	for i,b := range buf {
		state = nlDotNl_transition(state,b)
		if state == nlDotNl_end {
			d.data = buf[:i+1]
			d.r.b.advanceWrite(i+1)
			d.end = true
			return
		}
	}
	// Put state back.
	d.state = state
	d.data = buf
	d.r.b.reset()
}
func (d *DotReader) Read(b []byte) (int,error) {
	d.innerRead()
	e := d.err
	buf := d.data
	if len(buf) > len(b) {
		copy(b,buf)
		d.data = buf[len(b):]
		return len(b),nil
	}
	if len(buf) > 0 {
		copy(b,buf)
		d.data = nil
		if d.end { return len(buf),io.EOF }
		return len(buf),nil
	}
	if e==nil { e = io.EOF }
	return 0,e
}
// Consumes until end.
func (d *DotReader) Consume() {
	for {
		d.innerRead()
		if d.end || len(d.data) == 0 { return }
		d.data = nil
	}
}
func (r *Reader) DotReader() (d *DotReader) {
	d = pool_DotReader.Get().(*DotReader)
	*d = DotReader{
		r: r,
		state: nlDotNl_start,
		data: nil,
		end: false,
		err: nil,
	}
	return d
}
var pool_DotReader = sync.Pool{ New : func() interface{} { return new(DotReader) }}
//func AcquireDotReader() *DotReader {
//	return pool_DotReader.Get().(*DotReader)
//}
func (r *DotReader) Release() {
	// Cut-off references to objects, we might want to be garbage-collected.
	r.r = nil
	r.data = nil
	r.err = nil
	pool_DotReader.Put(r)
}

