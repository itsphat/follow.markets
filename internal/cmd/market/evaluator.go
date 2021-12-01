package market

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dlclark/regexp2"

	"follow.market/internal/pkg/strategy"
	tax "follow.market/internal/pkg/techanex"
	"follow.market/pkg/log"
)

type evaluator struct {
	sync.Mutex
	connected bool
	signals   *sync.Map

	// shared properties with other market participants
	logger       *log.Logger
	provider     *provider
	communicator *communicator
}

type emember struct {
	name    string
	regex   []*regexp2.Regexp
	tChann  chan *tax.Trade
	signals strategy.Signals
}

func newEvaluator(participants *sharedParticipants) (*evaluator, error) {
	if participants == nil || participants.communicator == nil || participants.logger == nil {
		return nil, errors.New("missing shared participants")
	}
	e := &evaluator{
		connected: false,
		signals:   &sync.Map{},

		logger:       participants.logger,
		provider:     participants.provider,
		communicator: participants.communicator,
	}
	return e, nil
}

func (e *evaluator) connect() {
	e.Lock()
	defer e.Unlock()
	if e.connected {
		return
	}
	go func() {
		for msg := range e.communicator.watcher2Evaluator {
			go e.processingWatcherRequest(msg)
		}
	}()
	go func() {
		for msg := range e.communicator.streamer2Evaluator {
			go e.processStreamerRequest(msg)
		}
	}()
	e.connected = true
}

// add adds a new signal to the evalulator. The evaluator will evaluate the signal
// every minute on all tickers that satisfied the given patterns.
func (e *evaluator) add(patterns []string, s *strategy.Signal) error {
	var mem emember
	val, ok := e.signals.Load(s.Name)
	if !ok {
		reges := make([]*regexp2.Regexp, 0)
		for _, t := range patterns {
			reg, err := regexp2.Compile(t, 0)
			if err != nil {
				return err
			}
			reges = append(reges, reg)
		}
		mem = emember{
			name:    s.Name,
			regex:   reges,
			tChann:  make(chan *tax.Trade),
			signals: strategy.Signals{s},
		}
		e.signals.Store(s.Name, mem)
	} else {
		mem = val.(emember)
		mem.signals = append(mem.signals, s)
		e.signals.Store(s.Name, mem)
	}
	if s.IsOnTrade() {
		go e.await(mem, s)
	}
	return nil
}

// get returns a slice of signal that are applicable to the given ticker.
func (e *evaluator) get(ticker string) strategy.Signals {
	out := strategy.Signals{}
	e.signals.Range(func(k, v interface{}) bool {
		m := v.(emember)
		for _, re := range m.regex {
			if isMatched, err := re.MatchString(ticker); err == nil && isMatched {
				out = append(out, m.signals...)
			}
		}
		return true
	})
	return out
}

func (e *evaluator) await(mem emember, s *strategy.Signal) {
	for !e.registerStreamingChannel(mem) {
		e.logger.Error.Println(e.newLog(mem.name, "failed to register streaming data"))
	}
	go func() {
		for msg := range mem.tChann {
			if s.Evaluate(nil, msg) {
				e.communicator.evaluator2Notifier <- e.communicator.newMessage(s, nil)
			}
		}
	}()
}

func (e *evaluator) registerStreamingChannel(mem emember) bool {
	doneStreamingRegister := false
	var maxTries int
	for !doneStreamingRegister && maxTries <= 3 {
		resC := make(chan *payload)
		e.communicator.evaluator2Streamer <- e.communicator.newMessage(mem, resC)
		doneStreamingRegister = (<-resC).what.(bool)
		maxTries++
	}
	return doneStreamingRegister
}

func (e *evaluator) processingWatcherRequest(msg *message) {
	r := msg.request.what.(wmember).runner
	signals := e.get(r.GetName())
	for _, s := range signals {
		if s.Evaluate(r, nil) {
			e.communicator.evaluator2Notifier <- e.communicator.newMessage(s, nil)
		}
	}
}

func (e *evaluator) processStreamerRequest(msg *message) {
	if mem, ok := e.signals.Load(msg.request.what.(string)); ok && msg.response != nil {
		msg.response <- e.communicator.newPayload(mem)
		close(msg.response)
	}
}

func (e *evaluator) newLog(ticker, message string) string {
	return fmt.Sprintf("[evaluator] %s: %s", ticker, message)
}
