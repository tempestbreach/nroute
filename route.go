package nroute

import(
    "fmt"
    "strings"

    "github.com/tempestbreach/augnats"
    "github.com/nats-io/nats.go"
)

type Route struct {
    handler     augnats.Handler
    path        string
    err         error
    routeConf
}

//--------------------
// Matchers
//--------------------

type matcher interface{
    Match(*nats.Msg, *RouteMatch) bool
}

func(r *Route) addMatcher(m matcher) *Route {
    if r.err == nil {
        r.matchers = append(r.matchers, m)
    }
    return r
}

func(r *Route) addRegexpMatcher(tpl string, typ regexpType) error {
    if r.err != nil {
        return r.err
    }
    if typ == regexpTypePath || typ == regexpTypePrefix {
        if len(tpl) > 0 && tpl[0] != '.' {
            return fmt.Errorf("nroute: path must start with a dot, got %q", tpl)
        }
        if r.regexp.path != nil {
            tpl = strings.TrimRight(r.regexp.path.template, ".") + tpl
        }
    }
    rr, err := newRouteRegexp(tpl, typ, routeRegexpOptions{
        strictDot:      r.strictDot,
    })
    if err != nil {
        return err
    }
    if typ == regexpTypeBase {
        if r.regexp.path != nil {
            if err = uniqueVars(rr.varsN, r.regexp.path.varsN); err != nil {
                return err
            }
        }
        r.regexp.base = rr
    } else {
        if r.regexp.base != nil {
            if err = uniqueVars(rr.varsN, r.regexp.path.varsN); err != nil {
                return err
            }
        }
        r.regexp.path = rr
    }
    r.addMatcher(rr)
    return nil
}

func(r *Route) HandlerFunc(f func(*nats.Msg)) *Route {
    return r.Handler(augnats.HandlerFunc(f))
}

func(r *Route) Handler(handler augnats.Handler) *Route {
    r.handler = handler
    return r
}

func(r *Route) Match(msg *nats.Msg, match *RouteMatch) bool {
    // if msg.Subject == r.path {
    //     match.Handler = r.handler
    //     return true
    // }
    // return false

    var matchErr error

    for _, m := range r.matchers {
        if matched := m.Match(msg, match); !matched {
            if match.MatchErr == ErrNotFound {
                match.MatchErr = nil
            }

            matchErr = nil
            return false
        }
    }

    if matchErr != nil {
        match.MatchErr = matchErr
        return false
    }

    if match.Route == nil {
        match.Route = r
    }
    if match.Handler == nil {
        match.Handler = r.handler
    }
    if match.Vars == nil {
        match.Vars = make(map[string]string)
    }

    r.regexp.setMatch(msg, match, r)
    return true
}

func(r *Route) Path(tpl string) *Route {
    // r.path = path
    // r.err = r.addMatcher(path)
    r.err = r.addRegexpMatcher(tpl, regexpTypePath)
    return r
}

func(r *Route) PathPrefix(tpl string) *Route {
    r.err = r.addRegexpMatcher(tpl, regexpTypePrefix)
    return r
}

type BuildVarsFunc func(map[string]string) map[string]string

func (r *Route) BuildVarsFunc(f BuildVarsFunc) *Route {
	if r.buildVarsFunc != nil {
		// compose the old and new functions
		old := r.buildVarsFunc
		r.buildVarsFunc = func(m map[string]string) map[string]string {
			return f(old(m))
		}
	} else {
		r.buildVarsFunc = f
	}
	return r
}

func(r *Route) Subrouter() *Router {
    router := &Router{routeConf: copyRouteConf(r.routeConf)}
    r.addMatcher(router)
    return router
}
