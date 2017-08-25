/*
MIT License

Copyright (c) 2017 Simon Schmidt
Copyright (c) 2012-2014  Dustin Sallings <dustin@spy.net>

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

const crlf = "\r\n"

func splitWS(buf []byte, ext [][]byte) [][]byte {
	j := 0
	for i,b := range buf {
		if isWhiteSpace(b) {
			if i>j {
				ext = append(ext,buf[j:i])
			}
			j = i+1
		}
	}
	if j<len(buf) {
		ext = append(ext,buf[j:])
	}
	if len(ext)==0 {
		ext = append(ext,nil)
	}
	return ext
}
func aToLower(buf []byte) {
	for i,b := range buf {
		buf[i] = toLower(b)
	}
}
func beq(a,b []byte) bool {
	if len(a)!=len(b) { return false }
	for i,c := range a {
		if b[i]!=c { return false }
	}
	return true
}

var pool_nntpHandler = sync.Pool{ New: func() interface{} {
	return &nntpHandler{
	end: false,
	group: nil,
	lineBuffer : make([]byte,0,1<<13),
	outBuffer  : make([]byte,0,1<<13),
	groupBuffer: make([]byte,0,1<<13),
	idBuffer   : make([]byte,0,1<<7),
	}
}}

func (h *Handler) ServeConn(conn io.ReadWriteCloser) error {
	h.fill()
	nh := pool_nntpHandler.Get().(*nntpHandler)
	defer nh.release()
	defer conn.Close()
	rdr := AcquireReader().Init(conn)
	defer rdr.Release()
	nh.r = rdr
	nh.w = conn
	nh.h = h
	return nh.servceConn()
}

type nntpHandler struct {
	r *Reader
	w io.Writer
	h *Handler
	end bool
	group *Group
	groupCursor int64
	groupCurId  []byte
	
	lineBuffer  []byte
	outBuffer   []byte
	groupBuffer []byte
	idBuffer    []byte
}
func (h *nntpHandler) release() {
	if h==nil { return }
	h.r = nil
	h.w = nil
	h.h = nil
	if h.group!=nil { pool_Group_put(h.group) }
	h.group = nil
	pool_nntpHandler.Put(h)
}

type handleFunc func(h *nntpHandler,args [][]byte) error
var nntpCommands = map[string]handleFunc{
	""         :handleDefault,
	"quit"     :handleQuit,
	
	// RFC-3977    6.1.   Group and Article Selection 
	"listgroup":handleListgroup,
	"group"    :handleGroup,
	"last"     :handleLast,
	"next"     :handleNext,
	
	// RFC-3977    6.2.   Retrieval of Articles and Article Sections
	
}

func (h *nntpHandler) servceConn() error {
	h.end = false
	buffer := make([][]byte,0,10)
	h.writeMessage(200,"Hello!")
	for {
		line,err := h.r.ReadLineB(h.lineBuffer)
		if err!=nil { return err }
		args := splitWS(trimLeft(line),buffer)
		aToLower(args[0])
		
		handler,ok := nntpCommands[string(args[0])]
		if !ok {
			handler,ok = nntpCommands[""]
		}
		if !ok {
			panic("No default handler")
		}
		err = handler(h,args[1:])
		if err!=nil { return err }
		if h.end { return nil }
	}
	panic("unreachable")
}
func (h *nntpHandler) writeError(ne *NNTPError) error {
	out := h.outBuffer
	out = AppendUint(out,int64(ne.Code))
	out = append(out,' ')
	out = append(out,ne.Msg...)
	out = append(out,crlf...)
	_,e := h.w.Write(out)
	return e
}
func (h *nntpHandler) writeMessage(code int64, msg string) error {
	out := h.outBuffer
	out = AppendUint(out,code)
	out = append(out,' ')
	out = append(out,msg...)
	out = append(out,crlf...)
	_,e := h.w.Write(out)
	return e
}
func (h *nntpHandler) writeGroupF(w io.Writer,code int64,grp *Group) error {
	out := h.outBuffer
	out = AppendUint(out,code)
	out = append(out,' ')
	out = AppendUint(out,grp.Number)
	out = append(out,' ')
	out = AppendUint(out,grp.Low)
	out = append(out,' ')
	out = AppendUint(out,grp.High)
	out = append(out,' ')
	out = append(out,grp.Group...)
	out = append(out,crlf...)
	_,e := w.Write(out)
	return e
}

func handleDefault(h *nntpHandler,args [][]byte) error {
	return h.writeError(ErrUnknownCommand)
}

func handleQuit(h *nntpHandler,args [][]byte) error {
	h.end = true
	return h.writeMessage(205,"bye")
}


// RFC-3977    6.1.   Group and Article Selection 

/*
   Indicating capability: READER
   
   Syntax
     LISTGROUP [group [range]]
     
   Responses
     211 number low high group     Article numbers follow (multi-line)
     411                           No such newsgroup
     412                           No newsgroup selected [1]
     
   Parameters
     group     Name of newsgroup
     range     Range of articles to report
     number    Estimated number of articles in the group
     low       Reported low water mark
     high      Reported high water mark
   [1] The 412 response can only occur if no group has been specified.
*/
func handleListgroup(h *nntpHandler,args [][]byte) error {
	grp := h.group
	arg0 := []byte(nil)
	arg1 := []byte(nil)
	if len(args)>0 {arg0 = args[0]}
	if len(args)>1 {arg1 = args[1]}
	if len(arg0)!=0 {
		ok := false
		if grp!=nil { ok = beq(grp.Group,arg0) }
		if grp==nil || !ok {
			grp = pool_Group.Get().(*Group)
			defer pool_Group_put(grp)
			if !h.h.GetGroup(grp) {
				return h.writeError(ErrNoSuchGroup)
			}
		}
	}
	if grp==nil {
		return h.writeError(ErrNoGroupSelected)
	}
	
	from, to := ParseRange(arg1)
	bw := AcquireBufferedWriter(h.w)
	defer func(){
		bw.Flush()
		ReleaseBufferedWriter(bw)
	}()
	
	err := h.writeGroupF(bw,211,grp)
	if err!=nil { return err }
	
	dw := AcquireDotWriter()
	dw.Reset(bw)
	defer func(){
		dw.Close()
		dw.Release()
	}()
	h.h.ListGroup(grp,dw,from,to)
	
	return nil
}

/*
   Indicating capability: READER
   
   Syntax
     GROUP group
     
   Responses
     211 number low high group     Group successfully selected
     411                           No such newsgroup
     
   Parameters
     group     Name of newsgroup
     number    Estimated number of articles in the group
     low       Reported low water mark
     high      Reported high water mark
*/
func handleGroup(h *nntpHandler,args [][]byte) error {
	if len(args) < 1 { return h.writeError(ErrNoSuchGroup) }
	
	ogrp := h.group
	ngrp := pool_Group.Get().(*Group)
	
	ngrp.Group = args[0]
	
	if h.h.GetGroup(ngrp) {
		ngrp.Group = append(h.groupBuffer,args[0]...)
		h.group = ngrp
		h.groupCursor = -1
		h.groupCurId = nil
		pool_Group_put(ogrp)
		return h.writeGroupF(h.w,211,ngrp)
	}else{
		pool_Group_put(ngrp)
		return h.writeError(ErrNoSuchGroup)
	}
	panic("unreachable")
}

/*
   Indicating capability: READER
   
   Syntax
     LAST
     
   Responses
     223 n message-id    Article found
     412                 No newsgroup selected
     420                 Current article number is invalid
     422                 No previous article in this group
     
   Parameters
     n             Article number
     message-id    Article message-id
     
Moves the current article pointer to the previous article.
*/
func handleLast(h *nntpHandler,args [][]byte) error {
	grp := h.group
	if grp == nil { return h.writeError(ErrNoGroupSelected) }
	cur := h.groupCursor
	if cur<0 {
		cur = grp.High+1
	}
	
	cur,id,ok := h.h.CursorMoveGroup(grp,cur,true,h.idBuffer)
	
	if !ok { return h.writeError(ErrNoPreviousArticle) }
	h.groupCursor = cur
	h.groupCurId = id
	
	out := h.outBuffer
	out = AppendUint(out,223)
	out = append(out,' ')
	out = AppendUint(out,cur)
	out = append(out,' ')
	out = append(out,id...)
	out = append(out,crlf...)
	_,e := h.w.Write(out)
	return e
}

/*
   Indicating capability: READER
   
   Syntax
     NEXT
     
   Responses
   
     223 n message-id    Article found
     412                 No newsgroup selected
     420                 Current article number is invalid
     421                 No next article in this group
     
   Parameters
     n             Article number
     message-id    Article message-id
     
Moves the current article pointer to the next article.
*/
func handleNext(h *nntpHandler,args [][]byte) error {
	grp := h.group
	if grp == nil { return h.writeError(ErrNoGroupSelected) }
	cur := h.groupCursor
	if cur<0 {
		cur = grp.Low-1
	}
	
	cur,id,ok := h.h.CursorMoveGroup(grp,cur,false,h.idBuffer)
	
	if !ok { return h.writeError(ErrNoNextArticle) }
	h.groupCursor = cur
	h.groupCurId = id
	
	out := h.outBuffer
	out = AppendUint(out,223)
	out = append(out,' ')
	out = AppendUint(out,cur)
	out = append(out,' ')
	out = append(out,id...)
	out = append(out,crlf...)
	_,e := h.w.Write(out)
	return e
}


// RFC-3977    6.2.   Retrieval of Articles and Article Sections

/*
   Syntax
     STAT message-id
     STAT number
     STAT
     
   Responses
   
   First form (message-id specified)
     223 0|n message-id    Article exists
     430                   No article with that message-id
     
   Second form (article number specified)
     223 n message-id      Article exists
     412                   No newsgroup selected
     423                   No article with that number
     
   Third form (current article number used)
     223 n message-id      Article exists
     412                   No newsgroup selected
     420                   Current article number is invalid
     
   Parameters
     number        Requested article number
     n             Returned article number
     message-id    Article message-id
If a article number is passed, the server should set the "current article pointer" to it.
*/
func handleStat(h *nntpHandler,args [][]byte) error {
	out := h.outBuffer
	out = AppendUint(out,223)
	out = append(out,' ')
	if len(args)==0 {
		if h.group == nil {
			return h.writeError(ErrNoGroupSelected)
		}
		if len(h.groupCurId) == 0 {
			return h.writeError(ErrNoCurrentArticle)
		}
		out = AppendUint(out,h.groupCursor)
		out = append(out,' ')
		out = append(out,h.groupCurId...)
		out = append(out,crlf...)
		_,e := h.w.Write(out)
		return e
	}
	use_num := isDigit(args[0][0])
	
	if use_num && h.group==nil { return h.writeError(ErrNoGroupSelected) }
	article := new(Article)
	if use_num {
		num   := ParseUint(args[0])
		hasid := num == h.groupCursor
		article.HasNum = true
		article.MessageId = h.groupCurId
		article.Group  = h.group.Group
		article.Number = num
		article.HasId  = hasid
	}else{
		article.HasNum = false
		article.HasId  = false
		article.Number = 0
		article.MessageId = args[0]
	}
	
	if use_num && !article.HasId {
		if !h.h.StatArticle(article) {
			return h.writeError(ErrInvalidArticleNumber)
		}
	}else if !h.h.StatArticle(article) {
		return h.writeError(ErrInvalidMessageID)
	}
	
	out = AppendUint(out,article.Number)
	out = append(out,' ')
	out = append(out,article.MessageId...)
	out = append(out,crlf...)
	
	_,e := h.w.Write(out)
	return e
}


/*
   Syntax
     $cmd message-id
     $cmd number
     $cmd
     
   First form (message-id specified)
     *** 0|n message-id    Headers follow (multi-line)
     430                   No article with that message-id
     
   Second form (article number specified)
     *** n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     423                   No article with that number
     
   Third form (current article number used)
     *** n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid
*/
func handleArticleInternal(h *nntpHandler,args [][]byte,code int64,head, body bool) error {
	out := h.outBuffer
	out = AppendUint(out,code)
	out = append(out,' ')
	use_nothing := len(args)==0
	use_num := false
	if !use_nothing { use_num = isDigit(args[0][0]) }
	
	article := new(Article)
	if use_nothing {
		if h.group == nil {
			return h.writeError(ErrNoGroupSelected)
		}
		if len(h.groupCurId) == 0 {
			return h.writeError(ErrNoCurrentArticle)
		}
		article.Group = h.group.Group
		article.Number = h.groupCursor
		article.MessageId = h.groupCurId
		article.HasId = true
		article.HasNum = true
	}else if use_num {
		if h.group == nil {
			return h.writeError(ErrNoGroupSelected)
		}
		num   := ParseUint(args[0])
		hasid := num == h.groupCursor
		article.HasNum = true
		article.MessageId = h.groupCurId
		article.Group  = h.group.Group
		article.Number = num
		article.HasId  = hasid
	}else{
		article.HasNum = false
		article.HasId  = false
		article.Number = 0
		article.MessageId = args[0]
	}
	
	w := h.h.GetArticle(article,head,body)
	if w==nil {
		if use_nothing {
			return h.writeError(ErrNoCurrentArticle)
		}else if use_num {
			return h.writeError(ErrInvalidArticleNumber)
		}else{
			return h.writeError(ErrInvalidMessageID)
		}
		panic("unreachable")
	}
	
	out = AppendUint(out,article.Number)
	out = append(out,' ')
	out = append(out,article.MessageId...)
	out = append(out,crlf...)
	
	bw := AcquireBufferedWriter(h.w)
	defer func(){
		bw.Flush()
		ReleaseBufferedWriter(bw)
	}()
	
	_,err := bw.Write(out)
	if err!=nil { return err }
	
	dw := AcquireDotWriter()
	dw.Reset(bw)
	defer func(){
		dw.Close()
		dw.Release()
	}()
	w(dw)
	
	return nil
}


/*
   Syntax
     HEAD message-id
     HEAD number
     HEAD
     
   First form (message-id specified)
     221 0|n message-id    Headers follow (multi-line)
     430                   No article with that message-id
     
   Second form (article number specified)
     221 n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     423                   No article with that number
     
   Third form (current article number used)
     221 n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid
*/
func handleHead(h *nntpHandler,args [][]byte) error {
	return handleArticleInternal(h,args,221,true,false)
}

/*
   Syntax
     BODY message-id
     BODY number
     BODY
     
   Responses
   
   First form (message-id specified)
     222 0|n message-id    Body follows (multi-line)
     430                   No article with that message-id
     
   Second form (article number specified)
     222 n message-id      Body follows (multi-line)
     412                   No newsgroup selected
     423                   No article with that number
     
   Third form (current article number used)
     222 n message-id      Body follows (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid
     
   Parameters
     number        Requested article number
     n             Returned article number
     message-id    Article message-id
*/
func handleBody(h *nntpHandler,args [][]byte) error {
	return handleArticleInternal(h,args,222,false,true)
}

/*
   Syntax
     ARTICLE message-id
     ARTICLE number
     ARTICLE
     
   Responses
   
   First form (message-id specified)
     220 0|n message-id    Article follows (multi-line)
     430                   No article with that message-id
     
   Second form (article number specified)
     220 n message-id      Article follows (multi-line)
     412                   No newsgroup selected
     423                   No article with that number
     
   Third form (current article number used)
     220 n message-id      Article follows (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid
     
   Parameters
     number        Requested article number
     n             Returned article number
     message-id    Article message-id
*/
func handleArticle(h *nntpHandler,args [][]byte) error {
	return handleArticleInternal(h,args,220,true,true)
}

/*
Subject:
From:
Date:
Message-ID:
References:
Bytes:
Lines:
Xref:full
.
*/

