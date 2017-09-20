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
import "io/ioutil"
import "sync"
import "math"

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
	// RFC-3977    5.   Session Administration Commands
	""         :handleDefault,
   "capabilities"  :handleCapabilities,
	"mode"     :handleMode,
	"quit"     :handleQuit,
	
	// RFC-3977    6.1.   Group and Article Selection 
	"listgroup":handleListgroup,
	"group"    :handleGroup,
	"last"     :handleLast,
	"next"     :handleNext,
	
	// RFC-3977    6.2.   Retrieval of Articles and Article Sections
	"article"  :handleArticle,
	"head"     :handleHead,
	"body"     :handleBody,
	"stat"     :handleStat,
	
	// RFC-3977    6.3.   Article Posting
	"post"     :handlePost,
	"ihave"    :handleIHave,
	
	// RFC-4644    2.     The STREAMING Extension
	"check"    :handleCheck,
	"takethis" :handleTakethis,
	
	// RFC-3977    8.     Article Field Access Commands
	"over"     :handleXOver,
	"xover"    :handleXOver,
	"hdr"      :handleXHdr,
	"xhdr"     :handleXHdr,
	
	// RFC-3977    7.     Information Commands
	"date"     :handleDate,
	"help"     :handleHelp,
	"newgroups":handleNewgroups,
	
	// RFC-3977    7.6.   The LIST Commands
	"list"     :handleList,
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
			grp.Group = arg0
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
		if ogrp!=nil { pool_Group_put(ogrp) }
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
	article := pool_Article.Get().(*Article)
	defer pool_Article_put(article)
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
	
	article := pool_Article.Get().(*Article)
	defer pool_Article_put(article)
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

// RFC-3977    6.3.   Article Posting

/*

   Indicating capability: POST

   This command MUST NOT be pipelined.

   Syntax
     POST

   Responses

   Initial responses
     340    Send article to be posted
     440    Posting not permitted

   Subsequent responses
     240    Article received OK
     441    Posting failed
*/

func handlePost(h *nntpHandler,args [][]byte) error {
	// TODO: Check Permissions
	if !h.h.CheckPost() { return h.writeError(ErrPostingNotPermitted) }
	if e := h.writeMessage(340, "Send article to be posted"); e!=nil { return e }
	dotr := h.r.DotReader()
	r,f := h.h.PerformPost(nil, dotr)
	io.Copy(ioutil.Discard,dotr) // Eat up excess data.
	
	if r||f { return h.writeError(ErrPostingFailed) }
	return h.writeMessage(240, "Article received OK")
}

/*
   Indicating capability: IHAVE

   This command MUST NOT be pipelined.

   Syntax
     IHAVE message-id

   Responses

   Initial responses
     335    Send article to be transferred
     435    Article not wanted
     436    Transfer not possible; try again later

   Subsequent responses
     235    Article transferred OK
     436    Transfer failed; try again later
     437    Transfer rejected; do not retry

   Parameters
     message-id    Article message-id
*/

func handleIHave(h *nntpHandler,args [][]byte) error {
	// TODO: Check Permissions
	if len(args)==0 { return h.writeError(ErrSyntax) }
	id := args[0]
	wanted,possible := h.h.CheckPostId(id)
	if !wanted { return h.writeError(ErrNotWanted) }
	if !possible { return h.writeError(ErrIHaveNotPossible) }
	if e := h.writeMessage(335, "Send article to be transferred"); e!=nil { return e }
	dotr := h.r.DotReader()
	rejected,failed := h.h.PerformPost(id, dotr)
	io.Copy(ioutil.Discard,dotr) // Eat up excess data.
	
	if rejected { return h.writeError(ErrIHaveRejected) }
	if failed { return h.writeError(ErrIHaveFailed) }
	return h.writeMessage(235, "Article transferred OK")
}


// RFC-4644    2.     The STREAMING Extension

/*
   Syntax
      CHECK message-id

   Responses
      238 message-id   Send article to be transferred
      431 message-id   Transfer not possible; try again later
      438 message-id   Article not wanted

   Parameters
      message-id = Article message-id

   The first parameter of the 238, 431, and 438 responses MUST be the
   message-id provided by the client as the parameter to CHECK.
*/
func handleCheck(h *nntpHandler,args [][]byte) error {
	// TODO: Check Permissions
	if len(args)==0 { return h.writeError(ErrSyntax) }
	id := args[0]
	code := int64(238)
	wanted,possible := h.h.CheckPostId(id)
	if !possible { code = 431 }
	if !wanted { code = 438 }
	out := h.outBuffer
	out = AppendUint(out,code)
	out = append(out,' ')
	out = append(out,id...)
	out = append(out,crlf...)
	_,err := h.w.Write(out)
	return err
}

/*
   A client MUST NOT use this command unless the server advertises the
   STREAMING capability or returns a 203 response to the MODE STREAM
   command.

   Syntax
      TAKETHIS message-id

   Responses
      239 message-id   Article transferred OK
      439 message-id   Transfer rejected; do not retry

   Parameters
      message-id = Article message-id

   The first parameter of the 239 and 439 responses MUST be the
   message-id provided by the client as the parameter to TAKETHIS.
*/
func handleTakethis(h *nntpHandler,args [][]byte) error {
	// TODO: Check Permissions
	if len(args)==0 { h.end = true; return h.writeError(ErrSyntax) }
	id := args[0]
	
	dotr := h.r.DotReader()
	r,f := h.h.PerformPost(id, dotr)
	io.Copy(ioutil.Discard,dotr) // Eat up excess data.
	
	code := int64(239)
	if r||f { code = 439 }
	
	out := h.outBuffer
	out = AppendUint(out,code)
	out = append(out,' ')
	out = append(out,id...)
	out = append(out,crlf...)
	_,err := h.w.Write(out)
	return err
}



// RFC-3977    8.     Article Field Access Commands

/*
   Indicating capability: $cmd

   Syntax
     $cmd message-id
     $cmd range
     $cmd

   Responses

   First form (message-id specified)
     ***    *** (multi-line)
     430    No article with that message-id

   Second form (range specified)
     ***    *** (multi-line)
     412    No newsgroup selected
     423    No articles in that range

   Third form (current article number used)
     ***    *** (multi-line)
     412    No newsgroup selected
     420    Current article number is invalid

   Parameters
     ***           ***
     range         Number(s) of articles
     message-id    Message-id of article
*/
func handleHeaders(h *nntpHandler,args [][]byte, okResponse string, mode int ) error {
	use_nothing := len(args)==0
	use_num := false
	if !use_nothing { use_num = isDigit(args[0][0]) }
	
	article := pool_ArticleRange.Get().(*ArticleRange)
	defer pool_ArticleRange_put(article)
	if use_nothing {
		if h.group == nil {
			return h.writeError(ErrNoGroupSelected)
		}
		if len(h.groupCurId) == 0 {
			return h.writeError(ErrNoCurrentArticle)
		}
		article.Group = h.group.Group
		article.Number = h.groupCursor
		article.LastNumber = h.groupCursor
		article.MessageId = h.groupCurId
		article.HasId  = true
		article.HasNum = true
	}else  if use_num {
		if h.group == nil {
			return h.writeError(ErrNoGroupSelected)
		}
		num,last := ParseRange(args[0])
		if last<num || last==math.MaxInt64 { last = num }
		hasid := (num == h.groupCursor) && (num==last)
		article.HasNum = true
		article.MessageId = h.groupCurId
		article.Group  = h.group.Group
		article.Number = num
		article.LastNumber = last
		article.HasId  = hasid
	}else{
		article.HasNum = false
		article.HasId  = true
		article.Number = 0
		article.MessageId = args[0]
	}
	w := h.h.WriteOverview(article)
	if w==nil {
		if use_nothing {
			return h.writeError(ErrNoCurrentArticle)
		}else if use_num {
			return h.writeError(ErrInvalidArticleRange)
		}else{
			return h.writeError(ErrInvalidMessageID)
		}
		panic("unreachable")
	}
	
	bw := AcquireBufferedWriter(h.w)
	defer func(){
		bw.Flush()
		ReleaseBufferedWriter(bw)
	}()
	
	out := append(h.outBuffer,okResponse...)
	_,err := bw.Write(out)
	if err!=nil { return err }
	
	dw := AcquireDotWriter()
	dw.Reset(bw)
	ov := pool_Overview.Get().(*Overview)
	ov.reset(h.outBuffer,dw,mode)
	defer func(){
		ov.release()
		dw.Close()
		dw.Release()
	}()
	w(ov)
	
	return nil
}


/*
   Indicating capability: OVER

   Syntax
     OVER message-id
     OVER range
     OVER

   Responses

   First form (message-id specified)
     224    Overview information follows (multi-line)
     430    No article with that message-id

   Second form (range specified)
     224    Overview information follows (multi-line)
     412    No newsgroup selected
     423    No articles in that range

   Third form (current article number used)
     224    Overview information follows (multi-line)
     412    No newsgroup selected
     420    Current article number is invalid

   Parameters
     range         Number(s) of articles
     message-id    Message-id of article
*/

const handleXOver_conts = "224 Overview information follows (multi-line)\r\n"
func handleXOver(h *nntpHandler,args [][]byte) error {
	return handleHeaders(h,args,handleXOver_conts,0)
}

/*
   Indicating capability: HDR

   Syntax
     HDR field message-id
     HDR field range
     HDR field

   Responses

   First form (message-id specified)
     225    Headers follow (multi-line)
     430    No article with that message-id

   Second form (range specified)
     225    Headers follow (multi-line)
     412    No newsgroup selected
     423    No articles in that range

   Third form (current article number used)
     225    Headers follow (multi-line)
     412    No newsgroup selected
     420    Current article number is invalid

   Parameters
     field         Name of field
     range         Number(s) of articles
     message-id    Message-id of article
*/
const handleXHdr_conts = "225 Headers follow (multi-line)\r\n"
var handleXHdr_hdrs = map[string]int{
	"subject":1,
	"from"   :2,
	"date"   :3,
	"message-id": 4,
	"references": 5, "refs": 5, "ref": 5,
	"bytes": 6, ":bytes": 6,
	"lines": 7, ":lines": 7,
}
func handleXHdr(h *nntpHandler,args [][]byte) error {
	if len(args)==0 { h.writeError(ErrSyntax) }
	aToLower(args[0])
	num,ok := handleXHdr_hdrs[string(args[0])]
	
	// XXX: Correct error code for unknown header???
	if !ok { h.writeError(ErrSyntax) }
	
	return handleHeaders(h,args[1:],handleXOver_conts,num)
}

// RFC-3977    7.6.   The LIST Commands

const handleListGenericGroups_Resp = "215 list of newsgroups follows\r\n"
func handleListGenericGroups(h *nntpHandler,args [][]byte, mode ListActiveMode) error {
	var wm *WildMat
	wm = nil
	bw := AcquireBufferedWriter(h.w)
	defer func(){
		bw.Flush()
		ReleaseBufferedWriter(bw)
	}()
	
	if len(args)>0 { wm = ParseWildMatBinary(args[0]); if wm.Compile()!=nil { wm = nil } }
	
	_,err := bw.Write(append(h.outBuffer,handleListGenericGroups_Resp...))
	if err!=nil { return err }
	
	dw := AcquireDotWriter()
	dw.Reset(bw)
	ila := pool_ListActive.Get().(*ListActive)
	ila.reset(h.outBuffer,dw,mode,wm)
	defer func(){
		ila.release()
		dw.Close()
		dw.Release()
	}()
	
	h.h.ListGroups(wm,ila)
	
	return nil
}

/*
   LIST ACTIVE returns a list of valid newsgroups and associated
   information.  If no wildmat is specified, the server MUST include
   every group that the client is permitted to select with the GROUP
   command (Section 6.1.1).  Each line of this list consists of four
   fields separated from each other by one or more spaces:

   o  The name of the newsgroup.
   o  The reported high water mark for the group.
   o  The reported low water mark for the group.
   o  The current status of the group on this server.

      [C] LIST ACTIVE
      [S] 215 list of newsgroups follows
      [S] misc.test 3002322 3000234 y
      [S] comp.risks 442001 441099 m
      [S] alt.rfc-writers.recovery 4 1 y
      [S] tx.natives.recovery 89 56 y
      [S] tx.natives.recovery.d 11 9 n
      [S] .
*/
func handleListActive(h *nntpHandler,args [][]byte) error {
	return handleListGenericGroups(h,args,LAM_Active)
}

/*
   The newsgroups list is maintained by NNTP servers to contain the name
   of each newsgroup that is available on the server and a short
   description about the purpose of the group.  Each line of this list
   consists of two fields separated from each other by one or more space
   or TAB characters (the usual practice is a single TAB).  The first
   field is the name of the newsgroup, and the second is a short
   description of the group.  For example:

      [C] LIST NEWSGROUPS
      [S] 215 information follows
      [S] misc.test General Usenet testing
      [S] alt.rfc-writers.recovery RFC Writers Recovery
      [S] tx.natives.recovery Texas Natives Recovery
      [S] .
*/
func handleListNewsgroups(h *nntpHandler,args [][]byte) error {
	return handleListGenericGroups(h,args,LAM_Newsgroups)
}


func handleListOutputStrings(h *nntpHandler,args [][]byte,data []string) error {
	bw := AcquireBufferedWriter(h.w)
	defer func(){
		bw.Flush()
		ReleaseBufferedWriter(bw)
	}()
	
	_,err := bw.Write(append(h.outBuffer,handleList_resp...))
	if err!=nil { return err }
	
	dw := AcquireDotWriter()
	dw.Reset(bw)
	defer func(){
		dw.Close()
		dw.Release()
	}()
	
	for _,line := range data {
		dw.Write(append(h.outBuffer,line...))
	}
	
	return nil
}
/*

   The LIST OVERVIEW.FMT command returns a description of the fields in
   the database for which it is consistent (as described above).  The
   information is returned as a multi-line data block following the 215
   response code.  The information contains one line per field in the
   order in which they are returned by the OVER command; the first 7
   lines MUST (except for the case of letters) be exactly as follows:

       Subject:
       From:
       Date:
       Message-ID:
       References:
       :bytes
       :lines
*/
var handleListOverviewFmt_data = []string{
	"Subject:"+crlf,
	"From:"+crlf,
	"Date:"+crlf,
	"Message-ID:"+crlf,
	"References:"+crlf,
	":bytes"+crlf,
	":lines"+crlf,
}
func handleListOverviewFmt(h *nntpHandler,args [][]byte) error { return handleListOutputStrings(h,args,handleListOverviewFmt_data) }

/*

   Indicating capability: HDR

   Syntax
     LIST HEADERS [MSGID|RANGE]

   Responses
     215    Field list follows (multi-line)

   Parameters
     MSGID    Requests list for access by message-id
     RANGE    Requests list for access by range

   Example of an implementation providing access to only a few headers:

      [C] LIST HEADERS
      [S] 215 headers supported:
      [S] Subject
      [S] Message-ID
      [S] Xref
      [S] .
*/
var handleListHeaders_data = []string{
	"Subject"+crlf,
	"From"+crlf,
	"Date"+crlf,
	"Message-ID"+crlf,
	"References"+crlf,
	":bytes"+crlf,
	":lines"+crlf,
}
func handleListHeaders(h *nntpHandler,args [][]byte) error { return handleListOutputStrings(h,args,handleListHeaders_data) }


/*
   Indicating capability: LIST

   Syntax
     LIST [keyword [wildmat|argument]]

   Responses
     215    Information follows (multi-line)

   Parameters
     keyword     Information requested [1]
     argument    Specific to keyword
     wildmat     Groups of interest

   [1] If no keyword is provided, it defaults to ACTIVE.
*/
var handleList_map = map[string]handleFunc {
	"": handleListActive,
	"active": handleListActive,
	"newsgroups": handleListNewsgroups,
	"overview.fmt": handleListOverviewFmt,
	"headers": handleListHeaders,
}
const handleList_resp = "215 Information follows (multi-line)\r\n"
func handleList(h *nntpHandler,args [][]byte) error {
	kw := []byte(nil)
	if len(args)>0 {
		kw = args[0]
		args = args[1:]
	}
	aToLower(kw)
	hf,ok := handleList_map[string(kw)]
	if !ok { return h.writeError(ErrSyntax) }
	return hf(h,args)
}


