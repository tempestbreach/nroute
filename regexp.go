package nroute

import(
    "fmt"
    "bytes"
    "strings"
    "strconv"
    "regexp"

    "github.com/nats-io/nats.go"
)

type routeRegexpOptions struct {
    strictDot       bool
}

type regexpType int

const(
    regexpTypePath      regexpType = 0
    regexpTypeBase      regexpType = 1
    regexpTypePrefix    regexpType = 2
)

func newRouteRegexp(tpl string, typ regexpType, options routeRegexpOptions) (*routeRegexp, error) {
    idxs, errBraces := braceIndices(tpl)
    if errBraces != nil {
        return nil, errBraces
    }

    template := tpl

    defaultPattern := "[^.]+"
    if typ == regexpTypeBase {
        defaultPattern = "[^.]+"
    }

    if typ != regexpTypePath {
        options.strictDot = false
    }
    endDot := false
    if options.strictDot && strings.HasSuffix(tpl, ".") {
        tpl = tpl[:len(tpl) - 1]
        endDot = true
    }

    varsN := make([]string, len(idxs)/2)
    varsR := make([]*regexp.Regexp, len(idxs)/2)
    pattern := bytes.NewBufferString("")
    pattern.WriteByte('^')
    reverse := bytes.NewBufferString("")
    var end int
    var err error
    for i := 0; i < len(idxs); i += 2 {
        raw := tpl[end:idxs[i]]
        end = idxs[i+1]
        parts := strings.SplitN(tpl[idxs[i]+1:end-1], ":", 2)
        name := parts[0]
        patt := defaultPattern
        if len(parts) == 2 {
            patt = parts[1]
        }
        if name == "" || patt == "" {
            return nil, fmt.Errorf("nroute: missing name or pattern in %q",
                tpl[idxs[i]:end])
        }

        fmt.Fprintf(pattern, "%s(?P<%s>%s)", regexp.QuoteMeta(raw), varGroupName(i/2), patt)

        fmt.Fprintf(reverse, "%s%%s", raw)

        varsN[i/2] = name
        varsR[i/2], err = regexp.Compile(fmt.Sprintf("^%s$", patt))
        if err != nil {
            return nil, err
        }
    }

    raw := tpl[end:]
    pattern.WriteString(regexp.QuoteMeta(raw))
    if options.strictDot {
        pattern.WriteString("[.]?")
    }
    if typ != regexpTypePrefix {
        pattern.WriteByte('$')
    }
    reverse.WriteString(raw)
    if endDot {
        reverse.WriteByte('.')
    }

    reg, errCompile := regexp.Compile(pattern.String())
    if errCompile != nil {
        return nil, errCompile
    }

    return &routeRegexp{
        template:       template,
        regexpType:     typ,
        options:        options,
        regexp:         reg,
        reverse:        reverse.String(),
        varsN:          varsN,
        varsR:          varsR,
    }, nil
}

type routeRegexp struct {
    template        string
    regexpType      regexpType
    options         routeRegexpOptions
    regexp          *regexp.Regexp
    reverse         string
    varsN           []string
    varsR           []*regexp.Regexp
}

func(r *routeRegexp) Match(msg *nats.Msg, match *RouteMatch) bool {
    if r.regexpType == regexpTypeBase {
        base := getBase(msg, match.BaseDepth)
        return r.regexp.MatchString(base)
    }

    path := getPath(msg, match.BaseDepth)

    return r.regexp.MatchString(path)
}

func (r *routeRegexp) subject(vals map[string]string) (string, error) {
    subjectVals := make([]interface{}, len(r.varsN), len(r.varsN))
    for k, v := range r.varsN {
        val, ok := vals[v]
        if !ok {
            return "", fmt.Errorf("nroute: missing subject 'variable' %q", v)
        }
        subjectVals[k] = val
    }
    rv := fmt.Sprintf(r.reverse, subjectVals...)
    if !r.regexp.MatchString(rv) {
        for k, v := range r.varsN {
            if !r.varsR[k].MatchString(vals[v]) {
                return "", fmt.Errorf(
                    "nroute: variable %q doesn't match, expected %q", vals[v],
                    r.varsR[k].String())
            }
        }
    }
    return rv, nil
}

func braceIndices(s string) ([]int, error) {
	var level, idx int
	var idxs []int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			if level++; level == 1 {
				idx = i
			}
		case '}':
			if level--; level == 0 {
				idxs = append(idxs, idx, i+1)
			} else if level < 0 {
				return nil, fmt.Errorf("nroute: unbalanced braces in %q", s)
			}
		}
	}
	if level != 0 {
		return nil, fmt.Errorf("nroute: unbalanced braces in %q", s)
	}
	return idxs, nil
}

func varGroupName(idx int) string {
    return "v" + strconv.Itoa(idx)
}

type routeRegexpGroup struct {
    base        *routeRegexp
    path        *routeRegexp
}

func(v routeRegexpGroup) setMatch(msg *nats.Msg, m *RouteMatch, r *Route) {
	// Store host variables.
	if v.base != nil {
		base := getBase(msg, m.BaseDepth)
		matches := v.base.regexp.FindStringSubmatchIndex(base)
		if len(matches) > 0 {
			extractVars(base, matches, v.base.varsN, m.Vars)
		}
	}
	path := getPath(msg, m.BaseDepth)
	// Store path variables.
	if v.path != nil {
		matches := v.path.regexp.FindStringSubmatchIndex(path)
		if len(matches) > 0 {
			extractVars(path, matches, v.path.varsN, m.Vars)
		}
	}
}

func getBase(msg *nats.Msg, baseDepth int) string {
    s := strings.Split(msg.Subject, ".")
    ns := s[0:baseDepth - 1]
    base := strings.Join(ns, ".")
    return base
}

func getPath(msg *nats.Msg, baseDepth int) string {
    s := strings.Split(msg.Subject, ".")
    ns := s[baseDepth:]
    path := strings.Join(ns, ".")
    fPath := fmt.Sprintf(".%s", path)
    return fPath
}

func extractVars(input string, matches []int, names []string, output map[string]string) {
	for i, name := range names {
		output[name] = input[matches[2*i+2]:matches[2*i+3]]
	}
}
