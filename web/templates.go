package web

import "html/template"

const css = `
:root{--bg:#fbfaf7;--fg:#1d1c1a;--mute:#6b6760;--line:#e4e0d8;--accent:#7a3b12}
@media (prefers-color-scheme:dark){:root{--bg:#161513;--fg:#ebe7df;--mute:#a19b90;--line:#2c2a26;--accent:#e0a36b}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:18px/1.6 Georgia,"Times New Roman",serif}
main{max-width:42rem;margin:0 auto;padding:2rem 1rem 4rem}
header.site{border-bottom:1px solid var(--line);margin-bottom:2rem;padding-bottom:1rem}
header.site a{color:inherit;text-decoration:none}
h1,h2,h3{line-height:1.25}
a{color:var(--accent)}
.mute,.addr{color:var(--mute);font-size:.85rem}
.addr{font-family:ui-monospace,Menlo,Consolas,monospace;word-break:break-all}
ul.docs{list-style:none;padding:0}
ul.docs li{padding:.6rem 0;border-bottom:1px solid var(--line)}
article img{max-width:100%;height:auto}
pre{overflow-x:auto;background:var(--line);padding:.8rem;border-radius:4px;font-size:.85rem}
code{font-family:ui-monospace,Menlo,Consolas,monospace}
blockquote{margin:0;padding-left:1rem;border-left:3px solid var(--line);color:var(--mute)}
table{border-collapse:collapse}td,th{border:1px solid var(--line);padding:.3rem .5rem}
.badge{display:inline-block;font:12px ui-monospace,monospace;border:1px solid var(--line);padding:.1rem .4rem;border-radius:3px}
form.w label{display:block;margin:.8rem 0 .2rem;font-size:.9rem}
form.w input[type=text],form.w textarea,form.w select{width:100%;font:16px ui-monospace,monospace;padding:.4rem;background:var(--bg);color:var(--fg);border:1px solid var(--line)}
form.w textarea{min-height:24rem}
button{font:inherit;padding:.4rem 1rem;margin-top:.8rem}
.note{border:1px solid var(--line);padding:.8rem;margin:1rem 0}
`

var pages = template.Must(template.New("").Parse(`
{{define "top"}}<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.}}</title><style>` + css + `</style>{{end}}

{{define "index"}}{{template "top" "forum://"}}<link rel="alternate" type="text/markdown" href="/index.md"></head><body><main>
<header class="site"><h1>forum://</h1><p class="mute">Publications this host keeps a verified copy of.</p></header>
{{if .}}<ul class="docs">{{range .}}<li><a href="/{{.Address}}">{{if .Title}}{{.Title}}{{else}}(untitled){{end}}</a>
<div class="addr">forum://{{.Address}}</div></li>{{end}}</ul>{{else}}<p class="mute">Nothing here yet.</p>{{end}}
</main></body></html>{{end}}

{{define "publication"}}{{template "top" .Title}}
<link rel="alternate" type="application/atom+xml" href="/{{.Address}}/feed.atom" title="{{.Title}}">
<link rel="alternate" type="text/markdown" href="/{{.Address}}.md"></head><body><main>
<header class="site"><h1><a href="/{{.Address}}">{{.Title}}</a></h1>
{{if .Description}}<p>{{.Description}}</p>{{end}}
<div class="addr">forum://{{.Address}}</div>
<p class="mute"><span class="badge">verified</span> head {{.Sequence}} · <a href="/{{.Address}}/feed.atom">feed</a> · <a href="/{{.Address}}.md">Markdown</a>{{range .Pages}} · <a href="/{{$.Address}}/{{.Doc}}">{{.Title}}</a>{{end}}</p></header>
{{if .Posts}}<ul class="docs">{{range .Posts}}<li><a href="/{{$.Address}}/{{.Doc}}">{{.Title}}</a>
<div class="mute">{{.Date}}</div></li>{{end}}</ul>{{else}}<p class="mute">No posts yet.</p>{{end}}
</main></body></html>{{end}}

{{define "document"}}{{template "top" .Title}}<link rel="alternate" type="text/markdown" href="/{{.Address}}/{{.Doc}}.md"></head><body><main>
<header class="site"><a href="/{{.Address}}">{{.PubTitle}}</a></header>
<article><h1>{{.Title}}</h1>{{if .Date}}<p class="mute">{{.Date}}{{if .Edited}} · edited {{.Edited}}{{end}}</p>{{end}}
{{.HTML}}</article>
<hr><p class="addr">forum://{{.Address}}/{{.Doc}}<br>revision {{.Revision}}<br>
<span class="badge">verified</span> <a href="/{{.Address}}/{{.Doc}}.md">Markdown</a></p>
</main></body></html>{{end}}

{{define "error"}}{{template "top" "forum://"}}</head><body><main><h1>{{.Status}}</h1><p>{{.Message}}</p></main></body></html>{{end}}
`))
