package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	MessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smsgw_messages_total",
			Help: "Messages lifecycle counter by stage and lane",
		},
		[]string{"stage", "lane"}, // queued|sent|failed , normal|express
	)
)

func MustRegister(r prometheus.Registerer) {
	r.MustRegister(
		MessagesTotal,
	)
}
