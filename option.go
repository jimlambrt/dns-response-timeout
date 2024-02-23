package respwriter

import (
	"log/slog"
)

// Option defines a common functional options type which can be used in a
// variadic parameter pattern.
type Option func(interface{})

// applyOpts takes a pointer to the options struct as a set of default options
// and applies the slice of opts as overrides.
func applyOpts(opts interface{}, opt ...Option) {
	for _, o := range opt {
		if o == nil { // ignore any nil Options
			continue
		}
		o(opts)
	}
}

type generalOptions struct {
	withLogger *slog.Logger
}

func generalDefaults() generalOptions {
	return generalOptions{}
}

func getGeneralOpts(opt ...Option) generalOptions {
	opts := generalDefaults()
	applyOpts(&opts, opt...)
	return opts
}

// WithLogger allows you to specify an optional logger.
func WithLogger(l *slog.Logger) Option {
	return func(o interface{}) {
		if o, ok := o.(*generalOptions); ok {
			if !isNil(l) {
				o.withLogger = l
			}
		}
	}
}
