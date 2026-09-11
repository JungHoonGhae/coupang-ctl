package browser

import _ "embed"

// Embedded readers are private to Camofox; no Chrome extension is installed or loaded.

//go:embed order_page_reader.js
var orderPageReader string

//go:embed search_page_reader.js
var searchPageReader string

//go:embed order_document_poll.js
var orderDocumentPoll string

//go:embed login_document_poll.js
var loginDocumentPoll string

//go:embed category_page_reader.js
var categoryPageReader string
