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


package posting

import "time"

type Stamper interface{
	GetId(id_buf []byte) []byte
	PathSeg(buf []byte) []byte
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

func versionate(id []byte, ri int) []byte{
	var stor [5]byte
	//if ri<0 { ri = -ri }
	for i := range stor {
		stor[i] = alphabet[ri&63]
		ri >>= 6
	}
	return append(id,stor[:]...)
}

const msgIdDate = ".20060102.150405.999@"

/*
The Hostname implements a Stamper, based on the server's host name.
The Message-IDs it generates are purely based on a timestamp.
*/
type HostName string
func (h HostName) PathSeg(buf []byte) []byte {
	return append(append(buf,h...),'!')
}
func (h HostName) GetId(id_buf []byte) []byte {
	t := time.Now()
	id := append(id_buf,'<')
	id = versionate(id,t.Nanosecond())
	id = t.AppendFormat(id,msgIdDate)
	id = append(id,h...)
	id = append(id,'>')
	return id
}

