package web

import "html/template"

var writerPages = template.Must(template.Must(pages.Clone()).Parse(`
{{define "w-home"}}{{template "top" "forum:// writer"}}</head><body><main>
<header class="site"><h1>Writer</h1><p class="mute">Local only. Nothing here is reachable from other machines.</p></header>
{{if .Flash}}<div class="note">{{.Flash}}</div>{{end}}
<h2>Your publications</h2>
{{if .Owned}}<ul class="docs">{{range .Owned}}<li><a href="/w/{{.Address}}">{{if .Title}}{{.Title}}{{else}}(untitled){{end}}</a>
<div class="addr">forum://{{.Address}} · {{.Label}}</div></li>{{end}}</ul>{{else}}<p class="mute">None yet.</p>{{end}}
<h2>New publication</h2>
<form class="w" method="post" action="/w/new">
<input type="hidden" name="token" value="{{.Token}}">
<label>Title</label><input type="text" name="title" required>
<label>Description</label><input type="text" name="description">
<label>Salt (leave empty for the next counter; a custom salt is normalized: case, accents and spacing do not matter)</label><input type="text" name="salt">
<button>Create</button></form>
<h2>Follow a publication</h2>
<form class="w" method="post" action="/w/follow">
<input type="hidden" name="token" value="{{.Token}}">
<label>forum:// address</label><input type="text" name="address" required>
<label>Where to fetch it (http://host:port)</label><input type="text" name="from" required>
<button>Follow</button></form>
<form class="w" method="post" action="/w/sync"><input type="hidden" name="token" value="{{.Token}}"><button>Sync followed publications</button></form>
{{if .Events}}<h2>Verification log</h2><pre>{{range .Events}}{{.}}
{{end}}</pre>{{end}}
</main></body></html>{{end}}

{{define "w-pub"}}{{template "top" .Title}}</head><body><main>
<header class="site"><a href="/w">Writer</a> · <a href="/{{.Address}}">view</a><h1>{{.Title}}</h1>
<div class="addr">forum://{{.Address}}</div></header>
{{if .Flash}}<div class="note">{{.Flash}}</div>{{end}}
<h2>Documents</h2>
{{if .Docs}}<ul class="docs">{{range .Docs}}<li><a href="/w/{{$.Address}}/edit/{{.Doc}}">{{.Title}}</a>
<span class="mute">{{.Entry.Kind}} · {{.Date}}</span></li>{{end}}</ul>{{else}}<p class="mute">No documents yet.</p>{{end}}
<p><a href="/w/{{.Address}}/edit/new">Write a new post or page</a></p>
<h2>Add an image or file</h2>
<form class="w" method="post" action="/w/{{.Address}}/asset" enctype="multipart/form-data">
<input type="hidden" name="token" value="{{.Token}}"><input type="file" name="file" required><button>Upload</button></form>
<h2>Publication details</h2>
<form class="w" method="post" action="/w/{{.Address}}/meta">
<input type="hidden" name="token" value="{{.Token}}">
<label>Title</label><input type="text" name="title" value="{{.Title}}">
<label>Description</label><input type="text" name="description" value="{{.Description}}">
<label>Known copies, one per line (http://host:port of hosts that carry this publication)</label>
<textarea name="copies" style="min-height:6rem">{{range .Copies}}{{.}}
{{end}}</textarea>
<button>Save details</button></form>
<h2>Export</h2><p><a href="/forum/v0/{{.Address}}/bundle">Download the whole publication as one bundle</a></p>
</main></body></html>{{end}}

{{define "w-edit"}}{{template "top" "Edit"}}</head><body><main>
<header class="site"><a href="/w">Writer</a> · <a href="/w/{{.Address}}">{{.PubTitle}}</a></header>
<form class="w" method="post" action="/w/{{.Address}}/publish">
<input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="doc" value="{{.Doc}}">
{{if not .Doc}}<label>Kind</label><select name="kind"><option value="post">post</option><option value="page">page</option></select>{{end}}
<label>Title</label><input type="text" name="title" value="{{.Title}}" required>
<label>Markdown</label><textarea name="body">{{.Body}}</textarea>
<button>{{if .Doc}}Publish this revision{{else}}Publish{{end}}</button></form>
{{if .Doc}}<form class="w" method="post" action="/w/{{.Address}}/remove">
<input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="doc" value="{{.Doc}}">
<button>Remove from the publication</button></form>{{end}}
</main></body></html>{{end}}
`))
