/*
 * The MIT License (MIT)
 * 
 * Copyright (c) 2015 Simon Schmidt
 * 
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 * 
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 * 
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * <http://www.opensource.org/licenses/mit-license.php>
 */


package fastnntp

import (
	"fmt"
	"strings"
	"regexp"
	"bytes"
)

var wildMatParse = regexp.MustCompile(`\*|\?|[^\*\?]+`)

type WildMat struct{
	RuleSets []*WildMatRuleSet
}
func (wmrs *WildMat) Match(s []byte) bool {
	for _,rs := range wmrs.RuleSets {
		if rs.Match(s) { return true }
	}
	return false
}
func (wmrs *WildMat) MatchString(s string) bool {
	for _,rs := range wmrs.RuleSets {
		if rs.MatchString(s) { return true }
	}
	return false
}
func (wmrs *WildMat) Compile() error {
	for _,rs := range wmrs.RuleSets {
		e := rs.Compile()
		if e!=nil { return e }
	}
	return nil
}
func (wmrs *WildMat) String() string {
	var buf bytes.Buffer
	buf.WriteString("<")
	for _,rs := range wmrs.RuleSets {
		fmt.Fprint(&buf,rs,"; ")
	}
	buf.WriteString(">")
	return buf.String()
}

type WildMatRuleSet struct{
	Positive []string
	Negative []string
	PR *regexp.Regexp
	NR *regexp.Regexp
}
func (wmrs *WildMatRuleSet) Match(s []byte) bool{
	return wmrs.PR.Match(s) && !wmrs.NR.Match(s)
}
func (wmrs *WildMatRuleSet) MatchString(s string) bool{
	return wmrs.PR.MatchString(s) && !wmrs.NR.MatchString(s)
}

func (wmrs *WildMatRuleSet) Compile() (e error){
	var buf bytes.Buffer
	compileToRegexp(&buf,wmrs.Positive)
	wmrs.PR,e = regexp.Compile(buf.String())
	if e!=nil { return }
	buf.Truncate(0)
	compileToRegexp(&buf,wmrs.Negative)
	wmrs.NR,e = regexp.Compile(buf.String())
	return
}

func compileToRegexp(buf *bytes.Buffer, wmts []string){
	begin := true
	buf.WriteString("^(")
	for _,wmt := range wmts {
		if begin {
			begin=false
		} else {
			buf.WriteString("|")
		}
		compileToRegexpPart(buf,wmt)
	}
	buf.WriteString(")$")
}

func compileToRegexpPart(buf *bytes.Buffer, wmt string){
	for _,wm := range wildMatParse.FindAllStringSubmatch(wmt,-1) {
		s := wm[0]
		switch s[0] {
		case '*':
			buf.WriteString(`.*`)
		case '?':
			buf.WriteString(`.`)
		default:
			buf.WriteString(regexp.QuoteMeta(s))
		}
	}
}

func ParseWildMat(wm string) *WildMat{
	wmr := new(WildMatRuleSet)
	wmra := []*WildMatRuleSet{wmr}
	elems := strings.Split(wm,",")
	positive := true
	for _,elem := range elems {
		if elem=="" { continue }
		if elem[0]=='!' {
			if positive { positive=false }
			wmr.Negative = append(wmr.Negative,elem[1:])
		} else {
			if !positive {
				wmr = new(WildMatRuleSet)
				wmra = append(wmra,wmr)
				positive=true
			}
			wmr.Positive = append(wmr.Positive,elem)
		}
	}
	return &WildMat{wmra}
}

func ParseWildMatBinary(wm []byte) *WildMat{
	wmr := new(WildMatRuleSet)
	wmra := []*WildMatRuleSet{wmr}
	elems := bytes.Split(wm,[]byte(","))
	positive := true
	for _,elem := range elems {
		if len(elem)==0 { continue }
		if elem[0]=='!' {
			if positive { positive=false }
			wmr.Negative = append(wmr.Negative,string(elem[1:]))
		} else {
			if !positive {
				wmr = new(WildMatRuleSet)
				wmra = append(wmra,wmr)
				positive=true
			}
			wmr.Positive = append(wmr.Positive,string(elem))
		}
	}
	return &WildMat{wmra}
}

