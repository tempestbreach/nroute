package nroute

import(
    "github.com/tempestbreach/augnats"
)

type MiddlewareFunc func(augnats.Handler) augnats.Handler

type middleware interface {
    Middleware(handler augnats.Handler) augnats.Handler
}

func(mw MiddlewareFunc) Middleware(handler augnats.Handler) augnats.Handler {
    return mw(handler)
}

func(r *Router) Use(mwf ...MiddlewareFunc) {
    for _, fn := range mwf {
        r.middlewares = append(r.middlewares, fn)
    }
}

func(r *Router) useInterface(mw middleware) {
    r.middlewares = append(r.middlewares, mw)
}
