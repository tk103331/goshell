package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestParseProgramReport(t *testing.T) {
	msg := b64("Apply 3 to add, 1 to change, 0 to destroy?")
	rep, ok := parseProgramReport("state=blocked:kind=permission:app=terraform:msg=" + msg + ":progress=40")
	if !ok {
		t.Fatal("report was discarded")
	}
	if rep.state != statusBlocked || rep.kind != statusPermission || rep.app != "terraform" || rep.msg != "Apply 3 to add, 1 to change, 0 to destroy?" || rep.progress != 40 {
		t.Fatalf("report = %+v", rep)
	}

	for _, body := range []string{
		"",                                   // no state
		"state=busy",                         // unknown state
		"state=working:id=a/b/c/d/e/f/g/h/i", // too deep
		"state=working:msg=" + b64("a\nb"),   // control character
		"state=working:msg=" + strings.Repeat("A", 2733), // too long
		"state=working:keytoolongkeytoolong=1",           // key over the limit
	} {
		if _, ok := parseProgramReport(body); ok {
			t.Errorf("%q was accepted", body)
		}
	}

	rep, ok = parseProgramReport("state=working:kind=auth:progress=200:app=bad/app")
	if !ok {
		t.Fatal("a bad optional value should not discard the report")
	}
	if rep.kind != "" || rep.progress != -1 || rep.app != "" {
		t.Fatalf("optional values = %+v", rep)
	}

	// base64 padding is optional
	rep, ok = parseProgramReport("state=done:msg=" + strings.TrimRight(b64("hi"), "="))
	if !ok || rep.msg != "hi" {
		t.Fatalf("unpadded msg: %+v %v", rep, ok)
	}
}

func TestProgramStatusRecords(t *testing.T) {
	st := newProgramStatus(nil, nil)
	st.apply(programReport{state: statusWorking, app: "deploy"})
	st.apply(programReport{id: "us-east", state: statusWorking, msg: "Pushing image", progress: 40})
	st.apply(programReport{id: "eu-west", state: statusBlocked, kind: statusPermission, msg: "Approve?"})

	sum, ok := st.summary()
	if !ok || sum.state != statusBlocked || sum.app != "deploy" || sum.msg != "Approve?" {
		t.Fatalf("summary = %+v %v", sum, ok)
	}

	st.apply(programReport{state: statusClear, id: "eu-west"})
	sum, ok = st.summary()
	if !ok || sum.state != statusWorking {
		t.Fatalf("after clear = %+v %v", sum, ok)
	}

	st.apply(programReport{id: "us-east", state: statusDone, msg: "Healthy"})
	st.markSeen()
	if sum, ok := st.summary(); !ok || sum.state != statusWorking {
		t.Fatalf("after seen = %+v %v, want the root working", sum, ok)
	}

	st.apply(programReport{state: statusClear})
	if _, ok := st.summary(); ok {
		t.Fatal("records survived a clear")
	}

	st.apply(programReport{id: "a/b", state: statusWorking})
	st.clearActive()
	if _, ok := st.summary(); ok {
		t.Fatal("working record survived a prompt")
	}
}
