package prioqueue

import "errors"

// ErrInvalidKind is returned by Queue.Push when kind is not a recognized
// request.Kind — neither request.KindPlain nor a Kind for which Kind.Valid
// reports true.
var ErrInvalidKind = errors.New("rcp/prioqueue: not a recognized request.Kind")
