package style

// uaCSS is the user-agent stylesheet, a small subset of the HTML spec's
// rendering section and Chromium's html.css.
const uaCSS = `
html, address, blockquote, body, dd, div, dl, dt, fieldset, form, frame,
frameset, h1, h2, h3, h4, h5, h6, noframes, ol, p, ul, center, dir, hr, menu,
pre, header, footer, nav, section, article, aside, main, figure, figcaption,
details, summary, hgroup, legend, search { display: block }
li { display: list-item }
head, style, title, meta, link, base, script, noscript, template, area, datalist, param { display: none }
[hidden] { display: none }
table { display: table; border-spacing: 2px; border-color: gray }
caption { display: table-caption; text-align: center }
thead { display: table-header-group; vertical-align: middle }
tbody { display: table-row-group; vertical-align: middle }
tfoot { display: table-footer-group; vertical-align: middle }
tr { display: table-row; vertical-align: inherit }
td, th { display: table-cell; vertical-align: inherit; padding: 1px }
th { font-weight: bold; text-align: -webkit-center }
col { display: table-column }
colgroup { display: table-column-group }
body { margin: 8px }
p, dl { margin-top: 1em; margin-bottom: 1em }
blockquote, figure { margin: 1em 40px }
h1 { font-size: 2em; margin-top: .67em; margin-bottom: .67em; font-weight: bold }
h2 { font-size: 1.5em; margin-top: .83em; margin-bottom: .83em; font-weight: bold }
h3 { font-size: 1.17em; margin-top: 1em; margin-bottom: 1em; font-weight: bold }
h4 { margin-top: 1.33em; margin-bottom: 1.33em; font-weight: bold }
h5 { font-size: .83em; margin-top: 1.67em; margin-bottom: 1.67em; font-weight: bold }
h6 { font-size: .67em; margin-top: 2.33em; margin-bottom: 2.33em; font-weight: bold }
ul, ol, menu, dir { margin-top: 1em; margin-bottom: 1em; padding-left: 40px }
dd { margin-left: 40px }
b, strong { font-weight: bold }
i, em, cite, var, dfn, address { font-style: italic }
u, ins { text-decoration: underline }
s, strike, del { text-decoration: line-through }
a:link { color: #0000ee; text-decoration: underline }
center { text-align: -webkit-center }
pre, code, kbd, samp, tt, xmp, listing, plaintext { font-family: monospace }
pre, xmp, listing, plaintext { white-space: pre; margin-top: 1em; margin-bottom: 1em }
small, sub, sup { font-size: smaller }
big { font-size: larger }
hr { border: 1px inset gray; margin: .5em auto; color: gray }
nobr { white-space: nowrap }
input, select, textarea, button { display: inline-block; font-size: 13.333px; border: 1px solid #767676; padding: 1px 2px; background-color: white; color: black }
input[type=hidden] { display: none }
input[type=submit], button { background-color: #efefef; padding: 1px 6px }
`

// quirksCSS holds the extra user-agent rules Chromium applies in quirks mode.
const quirksCSS = `
table { white-space: normal; line-height: normal; font-weight: normal; font-size: medium; font-style: normal; text-align: left }
form { margin-bottom: 1em }
`
