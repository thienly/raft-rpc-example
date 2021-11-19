package main

import (
	"raft-grpc-example/proto"
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/raft"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

type wordTracker struct {
	mtx sync.Mutex
	words [3]string
}

func (f *wordTracker) Apply(l *raft.Log) interface{} {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	w := string(l.Data)
	for i := 0; i < len(f.words); i++ {
		if compareWords(w, f.words[i]) {
			copy(f.words[i+1:], f.words[i:])
			f.words[i] = w
			break
		}
	}
	return nil
}
func (f *wordTracker) Snapshot() (raft.FSMSnapshot, error) {
	return &snapshot{words: cloneWords(f.words)}, nil
}

func (f *wordTracker) Restore(closer io.ReadCloser) error {
	b, err:= ioutil.ReadAll(closer)
	if err != nil {
		return err
	}
	words:= strings.Split(string(b), "\n")
	copy(f.words[:],words)
	return nil
}

var _ raft.FSM = &wordTracker{}

// compareWords returns true if a is longer (lexicography breaking ties).
func compareWords(a, b string) bool {
	if len(a) == len(b) {
		return a < b
	}
	return len(a) > len(b)
}

func cloneWords(words [3]string) []string {
	var ret [3]string
	copy(ret[:], words[:])
	return ret[:]
}

type snapshot struct {
	words []string
}

func (s snapshot) Persist(sink raft.SnapshotSink) error {
	_, err:= sink.Write([]byte(strings.Join(s.words,"\n")))
	if err != nil {
		sink.Cancel()
		return fmt.Errorf("sink.Write(): %v", err)
	}
	return sink.Close()
}

func (s snapshot) Release() {}


type rpcInterface struct {
	wordTracker *wordTracker
	raft *raft.Raft
	proto.UnimplementedExampleServer
}

func (r rpcInterface) AddWord(ctx context.Context, req *proto.AddWordRequest) (*proto.AddWordResponse, error) {
	f:= r.raft.Apply([]byte(req.GetWord()), time.Second)
	if err:= f.Error(); err != nil {
		return nil, errors.New("Can not add word")
	}
	return &proto.AddWordResponse{CommitIndex: f.Index()}, nil
}

func (r rpcInterface) GetWords(ctx context.Context, req *proto.GetWordsRequest) (*proto.GetWordsResponse, error) {
	r.wordTracker.mtx.Lock()
	defer r.wordTracker.mtx.Unlock()
	return &proto.GetWordsResponse{
		ReadAtIndex: r.raft.AppliedIndex(),
		BestWords:   cloneWords(r.wordTracker.words),
	}, nil
}