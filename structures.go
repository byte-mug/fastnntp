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

import "sync"

type Group struct{
	Group []byte
	Number int64
	Low int64
	High int64
}
var pool_Group = sync.Pool{New: func()interface{} { return new(Group) }}
func pool_Group_put(g *Group) {
	g.Group = nil
	pool_Group.Put(g)
}

type GroupCaps interface {
	GetGroup(g *Group) bool
	ListGroup(g *Group,w *DotWriter,first,last int64)
	CursorMoveGroup(g *Group,i int64,backward bool,id_buf []byte) (ni int64,id []byte,ok bool)
}

var pool_Article = sync.Pool{New: func()interface{} { return new(Article) }}
func pool_Article_put(g *Article) {
	g.MessageId = nil
	g.Group = nil
	pool_Article.Put(g)
}

type Article struct{
	MessageId []byte
	Group []byte
	Number int64
	HasId bool
	HasNum bool
}

var pool_ArticleRange = sync.Pool{New: func()interface{} { return new(ArticleRange) }}
func pool_ArticleRange_put(g *ArticleRange) {
	g.MessageId = nil
	g.Group = nil
	pool_ArticleRange.Put(g)
}

type ArticleRange struct{
	Article
	LastNumber int64
}
type ArticleCaps interface {
	// Every method (except WriteOverview) must set the message-id on success, if it is not given.
	
	StatArticle(a *Article) bool
	GetArticle(a *Article,head, body bool) func(w *DotWriter)
	WriteOverview(ar *ArticleRange) func(w IOverview)
}
type PostingCaps interface{
	CheckPostId(id []byte) (wanted bool, possible bool)
	CheckPost() (possible bool)
	PerformPost(id []byte, r *DotReader) (rejected bool,failed bool)
}

type GroupListingCaps interface{
	// Performs a List-Active action.
	// the argument 'wm' may be nil.
	ListGroups(wm *WildMat, ila IListActive) bool
}

// Privilege for a user.
type LoginPriv uint
const(
	LoginPriv_Post LoginPriv = iota
)

// NNTP Authentication.
// Documented outside RFC 3977 --> RFC 4643
type LoginCaps interface{
	// This Method SHOULD return true, if authentication has already occurred.
	AuthinfoDone(h *Handler) bool
	
	// Checks a privilege. Returns true if it is allowed.
	AuthinfoCheckPrivilege(p LoginPriv,h *Handler) bool
	
	// This Method returns true, if the combination of username is accepted without password.
	// The method can optionally return a new Handler object in place of the old one.
	AuthinfoUserOny(user string, oldh *Handler) (bool,*Handler)
	
	// This Method returns true, if the combination of username and password is accepted.
	AuthinfoUserPass(user, password string, oldh *Handler) (bool,*Handler)
}


type Handler struct {
	GroupCaps
	ArticleCaps
	PostingCaps
	GroupListingCaps
	LoginCaps
}
func (h *Handler) fill() {
	if h.GroupCaps==nil { h.GroupCaps = DefaultCaps }
	if h.ArticleCaps==nil { h.ArticleCaps = DefaultCaps }
	if h.PostingCaps==nil { h.PostingCaps = DefaultCaps }
	if h.GroupListingCaps==nil { h.GroupListingCaps = DefaultCaps }
	if h.LoginCaps==nil { h.LoginCaps = DefaultCaps }
}

var DefaultCaps = new(defCaps)

type defCaps struct {}
// GroupCaps
func (d *defCaps) GetGroup(g *Group) bool { return false }
func (d *defCaps) ListGroup(g *Group,w *DotWriter,first,last int64) { }
func (d *defCaps) CursorMoveGroup(g *Group,i int64,backward bool, id_buf []byte) (ni int64,id []byte,ok bool) { ok = false; return }

// ArticleCaps
func (d *defCaps) StatArticle(a *Article) bool { return false }
func (d *defCaps) GetArticle(a *Article,head, body bool) func(w *DotWriter) { return nil }
func (d *defCaps) WriteOverview(ar *ArticleRange) func(w IOverview) { return nil }

// PostingCaps
func (d *defCaps) CheckPostId(id []byte) (wanted bool, possible bool) { return }
func (d *defCaps) CheckPost() (possible bool) { return }
func (d *defCaps) PerformPost(id []byte, r *DotReader) (rejected bool,failed bool) { return true,true }

// GroupListingCaps
func (d *defCaps) ListGroups(wm *WildMat, ila IListActive) bool { return false }

// LoginCaps
func (d *defCaps) AuthinfoDone(h *Handler) bool { return true }

func (d *defCaps) AuthinfoCheckPrivilege(p LoginPriv,h *Handler) bool { return true }

func (d *defCaps) AuthinfoUserOny(user string, oldh *Handler) (bool,*Handler) { return false,nil }
	
func (d *defCaps) AuthinfoUserPass(user, password string, oldh *Handler) (bool,*Handler) { return false,nil }

