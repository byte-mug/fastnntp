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

import "fmt"
import "time"

// RFC-3977    5.   Session Administration Commands
/*
   This command is mandatory.

   Syntax
     CAPABILITIES [keyword]

   Responses
     101    Capability list follows (multi-line)
*/
var handleCapabilities_data = []string{
	"VERSION 2"+crlf,
	"READER"+crlf,
	"IHAVE"+crlf,
	"POST"+crlf,
	"LIST ACTIVE NEWSGROUPS OVERVIEW.FMT"+crlf,
	"OVER MSGID RANGE"+crlf,
	"HDR MSGID RANGE"+crlf,
	"STREAMING"+crlf,
}
func handleCapabilities(h *nntpHandler,args [][]byte) error {
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
	
	for _,line := range handleCapabilities_data {
		dw.Write(append(h.outBuffer,line...))
	}
	
	return nil
}

func handleModeReader(h *nntpHandler,args [][]byte) error {
	// TODO: Evaluate
	// h.writeMessage(200,"Posting allowed")
	// h.writeMessage(201,"Posting prohibited")
	// h.writeMessage(502,"Reading service permanently unavailable")
	return h.writeMessage(200,"Posting allowed")
}
var handleMode_map = map[string]handleFunc{
	"reader": handleModeReader,
}
func handleMode(h *nntpHandler,args [][]byte) error {
	if len(args)==0 { return h.writeError(ErrSyntax) }
	
	aToLower(args[0])
	hf,ok := handleMode_map[string(args[0])]
	if ok { return hf(h,args) }
	return h.writeError(ErrSyntax)
}

/*
   Indicating capability: READER

   Syntax
     DATE

   Responses
     111 yyyymmddhhmmss    Server date and time

   Parameters
     yyyymmddhhmmss    Current UTC date and time on server
*/


// RFC-3977    7.     Information Commands

func handleDate(h *nntpHandler,args [][]byte) error {
	t := time.Now().UTC()
	_,err := fmt.Fprintf(h.w,"111 %04d%02d%02d%%02d%02d%02d\r\n",t.Year(),t.Month(),t.Day(),t.Hour(),t.Minute(),t.Second())
	return err
}

/*
   This command is mandatory.

   Syntax
     HELP

   Responses
     100    Help text follows (multi-line)
*/
const handleHelp_text = "This is some help text."
func handleHelp(h *nntpHandler,args [][]byte) error {
	_,err := fmt.Fprintf(h.w,"100 Help text follows\r\n%s\r\n.\r\n",handleHelp_text)
	return err
}

/*
   Indicating capability: READER

   Syntax
     NEWGROUPS date time [GMT]

   Responses
     231    List of new newsgroups follows (multi-line)

   Parameters
     date    Date in yymmdd or yyyymmdd format
     time    Time in hhmmss format
*/
func handleNewgroups(h *nntpHandler,args [][]byte) error {
	out := append(h.outBuffer,"231 list of new newsgroups follows\r\n.\r\n"...) // Creation date is not available for any group.
	_,err := h.w.Write(out)
	return err
}

// Authentication support for NNTP

/*
 Documented outside RFC 3977 --> RFC 4643


   These commands MUST NOT be pipelined.

   Syntax
     AUTHINFO USER username
     AUTHINFO PASS password

   Responses
     281 Authentication accepted
     381 Password required [1]
     481 Authentication failed/rejected
     482 Authentication commands issued out of sequence
     502 Command unavailable [2]

     [1] Only valid for AUTHINFO USER.  Note that unlike traditional 3xx
         codes, which indicate that the client may continue the current
         command, the legacy 381 code means that the AUTHINFO PASS
         command must be used to complete the authentication exchange.

     [2] If authentication has already occurred, AUTHINFO USER/PASS are
         not valid commands (see Section 2.2).

         NOTE: Notwithstanding Section 3.2.1 of [NNTP], the server MUST
         NOT return 480 in response to AUTHINFO USER/PASS.

   Parameters
     username = string identifying the user/client
     password = string representing the user's password

------------------

   Example of successful AUTHINFO USER:

      [C] AUTHINFO USER wilma
      [S] 281 Authentication accepted

   Example of successful AUTHINFO USER/PASS:

      [C] AUTHINFO USER fred
      [S] 381 Enter passphrase
      [C] AUTHINFO PASS flintstone
      [S] 281 Authentication accepted

   Example of AUTHINFO USER/PASS requiring a security layer:

      [C] AUTHINFO USER fred@stonecanyon.example.com
      [S] 483 Encryption or stronger authentication required

   Example of failed AUTHINFO USER/PASS:

      [C] AUTHINFO USER barney
      [S] 381 Enter passphrase
      [C] AUTHINFO PASS flintstone
      [S] 481 Authentication failed

   Example of AUTHINFO PASS before AUTHINFO USER:

      [C] AUTHINFO PASS flintstone
      [S] 482 Authentication commands issued out of sequence

*/
var handleAuthInfo_keywords = map[string]int {
	"user" : 1,
	"pass" : 2,
}

const handleAuthInfo_281 = "281 Authentication accepted\r\n"
const handleAuthInfo_381 = "381 Enter passphrase\r\n"
const handleAuthInfo_481 = "481 Authentication failed/rejected\r\n"
const handleAuthInfo_482 = "482 Authentication commands issued out of sequence\r\n"
const handleAuthInfo_502 = "502 Command unavailable\r\n"
func handleAuthInfo(h *nntpHandler,args [][]byte) error {
	
	// AUTHINFO USER username
	// AUTHINFO PASS password
	// We require two arguments at this point.
	if len(args)<2 { return h.writeError(ErrSyntax) }
	
	if h.h.AuthinfoDone(h.h) {
		return h.writeRaw(append(h.outBuffer,handleAuthInfo_502...))
	}
	
	aToLower(args[0])
	switch handleAuthInfo_keywords[string(args[0])]{
	case 1:
		if ok,nh := h.h.AuthinfoUserOny(args[1],h.h); ok {
			if nh!=nil { h.h = nh }
			return h.writeRaw(append(h.outBuffer,handleAuthInfo_281...))
		}
		h.userName = append(h.userNameBuf,args[1]...)
		return h.writeRaw(append(h.outBuffer,handleAuthInfo_381...))
	case 2:
		if len(h.userName)==0 { return h.writeRaw(append(h.outBuffer,handleAuthInfo_482...)) }
		if ok,nh := h.h.AuthinfoUserPass(h.userName,args[1],h.h); ok {
			if nh!=nil { h.h = nh }
			return h.writeRaw(append(h.outBuffer,handleAuthInfo_281...))
		}
		return h.writeRaw(append(h.outBuffer,handleAuthInfo_481...))
	}
	_,err := fmt.Fprintf(h.w,"100 Help text follows\r\n%s\r\n.\r\n",handleHelp_text)
	return err
}

func (h *nntpHandler)issueCommandNotPermitted() error {
	// TODO: if h.h.AuthinfoDone(h.h) { ... }
	return h.writeError(ErrNotAuthenticated)
}

