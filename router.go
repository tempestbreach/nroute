package nroute

// Upstream and Downstream routes?

import(
    "fmt"
    "errors"
    "strings"
    // "context"

    "github.com/tempestbreach/augnats"
    "github.com/nats-io/nats.go"
)

var ErrNotFound = errors.New("no matching route was found")

func NewRouter(bs string) *Router {

    sl := strings.Split(bs, ".")
    baseDepth := len(sl)

    return &Router{
        namedRoutes: make(map[string]*Route),
        baseSubject: bs,
        baseDepth: baseDepth,
    }
}

type Router struct {
    NotFoundHandler     augnats.Handler
    routes              []*Route
    namedRoutes         map[string]*Route
    middlewares         []middleware
    baseSubject         string
    baseDepth               int
    KeepContext         bool
    routeConf
}

type routeConf struct {
    useEncodedPath          bool
    strictDot               bool
    regexp                  routeRegexpGroup
    matchers                []matcher
    buildScheme             string
    buildVarsFunc           BuildVarsFunc
}

func copyRouteConf(r routeConf) routeConf {
    c := r

    if r.regexp.path != nil {
        c.regexp.path = copyRouteRegexp(r.regexp.path)
    }

    if r.regexp.base != nil {
        c.regexp.base = copyRouteRegexp(r.regexp.base)
    }

    c.matchers = make([]matcher, len(r.matchers))
    copy(c.matchers, r.matchers)

    return c
}

func copyRouteRegexp(r *routeRegexp) *routeRegexp {
    c := *r
    return &c
}

func(r *Router) Match(msg *nats.Msg, match *RouteMatch) bool {
    for _, route := range r.routes {
        if route.Match(msg, match) {
            // Build middleware chain if no error was found
            if match.MatchErr == nil {
                for i := len(r.middlewares) - 1; i >= 0; i-- {
                    match.Handler = r.middlewares[i].Middleware(match.Handler)
                }
            }
            return true
        }
    }

    if r.NotFoundHandler != nil {
        match.Handler = r.NotFoundHandler
        match.MatchErr = ErrNotFound
        return true
    }

    match.MatchErr = ErrNotFound
    return false
}

// ---------------------------------------------------------
// Route factories
// ---------------------------------------------------------

func(r *Router) NewRoute() *Route {
    route := &Route{routeConf: copyRouteConf(r.routeConf)}
    r.routes = append(r.routes, route)
    return route
}

func(r *Router) Handle(path string, handler augnats.Handler) *Route {
    return r.NewRoute().Path(path).Handler(handler)
}

func(r *Router) HandleFunc(path string, f func(*nats.Msg)) *Route {
    return r.NewRoute().Path(path).HandlerFunc(f)
}

func(r *Router) HandleMsg(msg *nats.Msg) {

    var match RouteMatch
    match.BaseDepth = r.baseDepth
    var handler augnats.Handler
    if r.Match(msg, &match) {
        handler = match.Handler
        // msg = msgWithVars
        // msg = msgWithRoute
    }

    if handler == nil {
        handler = r.NotFoundHandler
    }

    handler.HandleMsg(msg)

}

func(r *Router) SetNotFoundHandler(handler augnats.Handler) {
    r.NotFoundHandler = handler
}

func(r *Router) BaseSubject(s string) {
    r.baseSubject = s


}

func(r *Router) Path(subj string) *Route {
    return r.NewRoute().Path(subj)
}

type RouteMatch struct {
    Route       *Route
    Handler     augnats.Handler
    Vars        map[string]string
    BaseDepth   int
    MatchErr    error
}

// func msgWithVars(r *nats.Msg, vars map[string]string) *nats.Msg {
// 	ctx := context.WithValue(r.Context(), varsKey, vars)
// 	return r.WithContext(ctx)
// }

// func msgWithRoute(msg *nats.Msg, route *Route) *nats.Msg {
// 	ctx := context.WithValue(r.Context(), routeKey, route)
// 	return r.WithContext(ctx)
// }

func (r *Router) BuildVarsFunc(f BuildVarsFunc) *Route {
	return r.NewRoute().BuildVarsFunc(f)
}

func uniqueVars(s1, s2 []string) error {
	for _, v1 := range s1 {
		for _, v2 := range s2 {
			if v1 == v2 {
				return fmt.Errorf("mux: duplicated route variable %q", v2)
			}
		}
	}
	return nil
}
