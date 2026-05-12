# SLO math

> Lands in phase 7 — recording rules + multi-window multi-burn-rate
> alerts. This file will derive the burn-rate thresholds from the
> chosen SLO, show the PromQL for each window, and explain why the
> 1h/6h pair (fast burn) and 1d/3d pair (slow burn) is the right
> shape per the SRE-book chapter on alerting.

Until then, the short version is:

- **orders SLO**: 99.5% of `POST /orders` requests return 2xx within
  300 ms over a rolling 30-day window. Error budget: 0.5% = ~3.6 h
  of badness per 30 days.
- **payments SLO**: 99.9% gRPC availability over 30 days.
- **inventory SLO**: same shape as payments.
- **email-worker SLO**: 99% of NATS messages acked within 60 s of
  arrival, measured at the queue end.

The multi-window pair fires fast on a real outage (1h window, 14.4×
burn) and slow on a creeping regression (6h window, 6× burn). The
two-window AND keeps page-fatigue low — neither false positive nor
late.
