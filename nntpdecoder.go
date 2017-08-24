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

func nntpcodec_split(buf []byte) ([]byte,[]byte) {
	for i,b := range buf {
		if b==' ' { return buf[:i],buf[i+1:] }
	}
	return buf,nil
}
func nntpcodec_lowerEq(data []byte,c string) bool {
	if len(data)!=len(c) { return false }
	for i,b := range data {
		if toLower(b)!=c[i] { return false }
	}
	return true
}

type nntpcodec struct {
	r *Reader
	w io.Writer
	h *Handler
}
func (n *nntpcodec) serveRequests() error {
	linebuf := make([]byte,0,300)
	outline := make([]byte,0,300)
	groupbuf := make([]byte,0,100)
	idbuf := make([]byte,0,100)
	group := new(Group)
	group_cursor := int64(0)
	cur_id := []byte{}
	//cur_valid := false
	article := new(Article)
	
	for {
		out := outline
		line,err := n.r.ReadLineB(linebuf)
		if err!=nil { return err }
		cmd,args := nntpcodec_split(trimLeft(line))
		args = trimRight(args)
		
		if nntpcodec_lowerEq(cmd,"group") {
			group.Group = append(groupbuf,args...)
			
			if n.h.GetGroup(group) {
				group_cursor = group.Low
				cur_id = nil
				//cur_valid = false
				out = append(out,'2','1','1',' ')
				out = AppendUint(out,group.Number)
				out = append(out,' ')
				out = AppendUint(out,group.Low)
				out = append(out,' ')
				out = AppendUint(out,group.High)
				out = append(out,' ')
				out = append(out,group.Group...)
				out = append(out,'\r','\n')
			}else{
				out = append(out,"411 No such newsgroup\r\n"...)
			}
			_,err = n.w.Write(out)
			if err!=nil { return err }
			continue
		}
		if nntpcodec_lowerEq(cmd,"listgroup") {
			agrp,arange := nntpcodec_split(args)
			arange = trimRight(arange)
			
			// TODO: arange
			
			if len(agrp)>0 {
				group.Group = append(groupbuf,args...)
				if !n.h.GetGroup(group) {
					group_cursor = group.Low
					cur_id = nil
					//cur_valid = false
					out = append(out,"411 No such newsgroup\r\n"...)
					_,err = n.w.Write(out)
					if err!=nil { return err }
					continue
				}
			}
			if len(group.Group)==0 {
				out = append(out,"412 No newsgroup selected\r\n"...)
				_,err = n.w.Write(out)
				if err!=nil { return err }
				continue
			}
			
			out = append(out,'2','1','1',' ')
			out = AppendUint(out,group.Number)
			out = append(out,' ')
			out = AppendUint(out,group.Low)
			out = append(out,' ')
			out = AppendUint(out,group.High)
			out = append(out,' ')
			out = append(out,group.Group...)
			out = append(out,'\r','\n')
			
			_,err = n.w.Write(out)
			if err!=nil { return err }
			dw := AcquireDotWriter()
			dw.Reset(n.w)
			n.h.ListGroup(group,dw)
			dw.Close()
			dw.Release()
			continue
		}
		if nntpcodec_lowerEq(cmd,"last")||nntpcodec_lowerEq(cmd,"next") {
			if len(group.Group)==0 {
				out = append(out,"412 No newsgroup selected\r\n"...)
				_,err = n.w.Write(out)
				if err!=nil { return err }
				continue
			}
			
			backward := false
			switch cmd[0] { case 'l','L':backward = true }
			
			nc,id,ok := n.h.CursorMoveGroup(group,group_cursor,backward)
			
			// XXX: "420 Current article number is invalid" ?
			if !ok {
				if backward {
					out = append(out,"422 No previous article in this group\r\n"...)
				}else{
					out = append(out,"421 No next article in this group\r\n"...)
				}
			}else{
				group_cursor = nc
				cur_id = append(idbuf,id...)
				//cur_valid = true
				out = append(out,'2','2','3',' ')
				out = AppendUint(out,nc)
				out = append(out,' ')
				out = append(out,id...)
				out = append(out,'\r','\n')
			}
			
			_,err = n.w.Write(out)
			if err!=nil { return err }
			continue
		}
		if nntpcodec_lowerEq(cmd,"article")||nntpcodec_lowerEq(cmd,"head")||nntpcodec_lowerEq(cmd,"body")||nntpcodec_lowerEq(cmd,"stat") {
			nc := int64(0)
			rform := 0
			rcmd := 0
			if len(args)==0 {
				rform = 1
			}else if (args[0] >= '0') && (args[0] <='9') {
				rform = 2
				nc = ParseUint(args)
			}else {
				rform = 3
			}
			switch cmd[0] {
			case 'a','A': rcmd = 'A'
			case 'h','H': rcmd = 'H'
			case 'b','B': rcmd = 'B'
			case 's','S': rcmd = 'S'
			}
			
			if (rform==1) && (rcmd=='S') {
				if len(cur_id) > 0 {
					out = append(out,'2','2','3',' ')
					out = AppendUint(out,group_cursor)
					out = append(out,' ')
					out = append(out,cur_id...)
					out = append(out,'\r','\n')
					_,err = n.w.Write(out)
					if err!=nil { return err }
					continue
				}
			}
			
			ok := true
			switch rform {
				case 1:
					ok = len(group.Group) != 0
					article.Group = group.Group
					article.Number = group_cursor
					article.HasNum = true
					article.MessageId = cur_id
					article.HasId = true
				case 2:
					ok = len(group.Group) != 0
					article.Group = group.Group
					article.Number = nc
					article.HasNum = true
				case 3:
					article.MessageId = cur_id
					article.HasId = true
			}
			if !ok { return nil }
			
			_,err = n.w.Write(out)
			if err!=nil { return err }
		}
	}
	return nil
}
