// Copyright Consensys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0
package zkc

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/consensys/go-corset/pkg/cmd/zkc/lsp"
	"github.com/consensys/go-corset/pkg/util/source"
	"github.com/consensys/go-corset/pkg/zkc/compiler"
	"github.com/spf13/cobra"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Start the zkc-language server.",
	Long:  `Start a Language Server Protocol (LSP) server for the zkc-language.`,
	Run: func(cmd *cobra.Command, args []string) {
		runLspServer(lspPort)
	},
}

var (
	// lspVerbose indicates whether to log to stderr or not.
	lspVerbose bool
	// lspPort is the TCP port to listen on. Zero means use stdio.
	lspPort uint16
	// lspLog is the path to the log file. Empty means no logging.
	lspLog string
)

// stdioConn wraps stdin/stdout into a single ReadWriteCloser for the JSON-RPC stream.
type stdioConn struct {
	io.Reader
	io.Writer
}

// Close implements io.Closer. Stdin/stdout are not closed; this is a no-op.
func (s stdioConn) Close() error { return nil }

// openLspLog opens the log file at path for appending and redirects the
// standard logger to it. If path is empty the logger is discarded. The caller
// is responsible for closing the returned file when done (nil is returned when
// path is empty).
func openLspLog(path string) *os.File {
	if path == "" && !lspVerbose {
		log.SetOutput(io.Discard)
		return nil
	} else if path == "" {
		log.SetOutput(os.Stderr)
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log file %s: %v\n", path, err)
		os.Exit(1)
	}

	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	return f
}

// runLspServer starts the LSP server. When port is zero it reads from stdin and
// writes to stdout; otherwise it listens for TCP connections on the given port,
// serving each connection as an independent LSP session.
func runLspServer(port uint16) {
	f := openLspLog(lspLog)
	if f != nil {
		//nolint
		defer f.Close()
	}

	if port == 0 {
		runLspConn(stdioConn{os.Stdin, os.Stdout})
		return
	}

	runLspServerTCP(port)
}

// runLspServerTCP listens on the given TCP port and spawns a goroutine to
// serve each accepted connection as an independent LSP session.
func runLspServerTCP(port uint16) {
	addr := fmt.Sprintf(":%d", port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen on %s: %v\n", addr, err)
	}

	log.Printf("listening on %s\n", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v\n", err)
			return
		}

		go runLspConn(conn)
	}
}

// runLspConn serves a single LSP session over the given ReadWriteCloser.
func runLspConn(rwc io.ReadWriteCloser) {
	ctx := context.Background()
	stream := jsonrpc2.NewStream(rwc)
	conn := jsonrpc2.NewConn(stream)
	handler := protocol.ServerHandler(&zkcServer{conn: conn}, jsonrpc2.MethodNotFoundHandler)
	conn.Go(ctx, handler)
	<-conn.Done()
}

// workspaceRoot extracts the workspace root directory from the parameters
// passed by the client at initialize time.  It prefers WorkspaceFolders (the
// modern LSP convention), falling back to the deprecated RootURI and then
// RootPath fields.  Returns the empty string when the client did not provide
// any of them, in which case no files are loaded up-front.
func workspaceRoot(params *protocol.InitializeParams) string {
	if len(params.WorkspaceFolders) > 0 {
		return uri.URI(params.WorkspaceFolders[0].URI).Filename()
	}

	// RootURI / RootPath are deprecated by LSP but still emitted by some
	// clients, so fall back to them when WorkspaceFolders is absent.
	//nolint:staticcheck
	if params.RootURI != "" {
		//nolint:staticcheck
		return params.RootURI.Filename()
	}
	//nolint:staticcheck
	return params.RootPath
}

// discoverZkcFiles walks root recursively and returns a ChangedFile update
// for each .zkc file it finds.  Files that cannot be read are logged and
// skipped; an empty root yields no updates.
func discoverZkcFiles(root string) []compiler.FileUpdate {
	if root == "" {
		return nil
	}

	var updates []compiler.FileUpdate

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("walk error at %s: %v", path, err)
			return nil
		}

		if d.IsDir() || filepath.Ext(path) != ".zkc" {
			return nil
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			log.Printf("zkc-lsp: cannot read %s: %v", path, err)
			return nil
		}

		updates = append(updates, compiler.ChangedFile(path, string(contents)))

		return nil
	})
	if err != nil {
		log.Printf("walk under %s failed: %v", root, err)
	}

	return updates
}

// zkcServer implements protocol.Server for the ZkC language.
type zkcServer struct {
	mu sync.RWMutex
	// compiler holds the in-memory view of every .zkc file the server is
	// tracking — those discovered under the workspace root at startup plus
	// any opened by the editor since.  It is recompiled in full on each
	// document change.
	compiler *compiler.IncrementalCompiler
	// dirtyDiagnostics is the set of file URIs for which the server most
	// recently published a non-empty diagnostics list.  When a subsequent
	// publish does not include such a file, an empty notification is sent
	// for it so that previously-displayed squiggles are cleared.
	dirtyDiagnostics map[protocol.URI]struct{}
	// pendingDiagnostics buffers errors discovered during Initialize so
	// they can be published from Initialized — the LSP spec forbids
	// arbitrary notifications before the Initialize response is sent.
	pendingDiagnostics []source.SyntaxError
	conn               jsonrpc2.Conn // retained so the server can push notifications to the client
}

// ============================================================================
// Lifecycle
// ============================================================================

// Initialize handles the LSP initialize request, which is the first message
// sent by a client before any other requests. The client describes its own
// capabilities and workspace configuration; the server responds with the set
// of features it supports. Clients must not send any further requests until
// they have received this response.
func (s *zkcServer) Initialize(
	_ context.Context, params *protocol.InitializeParams,
) (*protocol.InitializeResult, error) {
	s.mu.Lock()
	s.compiler = compiler.NewIncrementalCompiler()
	s.dirtyDiagnostics = make(map[protocol.URI]struct{})
	updates := discoverZkcFiles(workspaceRoot(params))
	s.pendingDiagnostics = s.compiler.Apply(updates...)
	s.mu.Unlock()

	log.Printf("initialised %d file(s) from workspace", len(updates))

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
			},
			SemanticTokensProvider: lsp.SemTokOptions{
				Legend: lsp.SemTokLegend,
				Full:   true,
			},
			HoverProvider:              true,
			DocumentSymbolProvider:     true,
			DefinitionProvider:         true,
			ReferencesProvider:         true,
			RenameProvider:             &protocol.RenameOptions{PrepareProvider: true},
			DocumentFormattingProvider: true,
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
		},
		ServerInfo: &protocol.ServerInfo{Name: "zkc"},
	}, nil
}

// Initialized handles the initialized notification sent by the client
// immediately after it has processed the initialize response. This marks the
// point at which the server may begin sending requests and notifications to
// the client (e.g. register dynamic capabilities, fetch configuration).
func (s *zkcServer) Initialized(ctx context.Context, _ *protocol.InitializedParams) error {
	s.mu.Lock()
	errs := s.pendingDiagnostics
	s.pendingDiagnostics = nil
	s.mu.Unlock()

	s.publishDiagnostics(ctx, errs)

	return nil
}

// Shutdown handles the shutdown request. The client asks the server to
// prepare for exit: the server must stop accepting new work, complete
// in-progress work, and free any held resources. The server must not exit
// yet; it waits for the subsequent exit notification before terminating.
// Responding to requests other than exit after shutdown should return an
// error.
func (s *zkcServer) Shutdown(_ context.Context) error {
	log.Printf("Shutdown")
	return nil
}

// Exit handles the exit notification. After receiving shutdown the client
// sends exit to signal that the server process should terminate. The server
// should exit with code 0 if a preceding shutdown request was received, or
// code 1 if the process is being forced to exit without a prior shutdown.
func (s *zkcServer) Exit(_ context.Context) error {
	os.Exit(0)
	return nil
}

// ============================================================================
// Notifications — accepted silently
// ============================================================================

var errNotImplemented = jsonrpc2.ErrMethodNotFound

// WorkDoneProgressCancel handles a $/cancelRequest notification sent by the
// client to cancel a work-done progress token that was previously created by
// the server via a window/workDoneProgress/create request. Upon receiving
// this notification the server should stop reporting progress for that token.
func (s *zkcServer) WorkDoneProgressCancel(
	_ context.Context, _ *protocol.WorkDoneProgressCancelParams,
) error {
	return nil
}

// LogTrace handles a $/logTrace notification sent by the client to request
// that the server append a trace message to its trace log. Trace messages
// are only sent when the trace level agreed during initialization is
// "messages" or "verbose".
func (s *zkcServer) LogTrace(_ context.Context, _ *protocol.LogTraceParams) error { return nil }

// SetTrace handles a $/setTrace notification, which instructs the server to
// change the verbosity of its trace output. The new value overrides the
// trace level negotiated during initialization and takes effect immediately.
func (s *zkcServer) SetTrace(_ context.Context, _ *protocol.SetTraceParams) error { return nil }

// publishDiagnostics groups the given syntax errors by source file and
// pushes a textDocument/publishDiagnostics notification per affected file.
//
// To avoid stale squiggles, files that previously had diagnostics but no
// longer appear in errs are sent an explicit empty notification: this is
// tracked via s.dirtyDiagnostics, which always reflects the set of files for
// which the server's last publish was non-empty.
func (s *zkcServer) publishDiagnostics(ctx context.Context, errs []source.SyntaxError) {
	byFile := lsp.DiagnosticsByFile(errs)

	s.mu.Lock()
	// Schedule a clear for any previously-dirty file that is now clean.
	for u := range s.dirtyDiagnostics {
		if _, still := byFile[u]; !still {
			byFile[u] = []protocol.Diagnostic{}
		}
	}
	// Refresh the dirty set to reflect what we are about to publish.
	s.dirtyDiagnostics = make(map[protocol.URI]struct{}, len(byFile))
	for u, diags := range byFile {
		if len(diags) > 0 {
			s.dirtyDiagnostics[u] = struct{}{}
		}
	}

	s.mu.Unlock()

	for u, diags := range byFile {
		params := &protocol.PublishDiagnosticsParams{URI: u, Diagnostics: diags}
		if err := s.conn.Notify(ctx, "textDocument/publishDiagnostics", params); err != nil {
			log.Printf("publishDiagnostics notify error: %v", err)
		}
	}
}

// DidOpen handles a textDocument/didOpen notification. The client sends this
// when a text document is opened in the editor, supplying the full document
// content. From this point the server is responsible for maintaining an
// up-to-date view of the document until the corresponding didClose is received.
func (s *zkcServer) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	uri := params.TextDocument.URI
	text := params.TextDocument.Text

	s.mu.Lock()
	errs := s.compiler.Apply(compiler.ChangedFile(uri.Filename(), text))
	s.mu.Unlock()

	s.publishDiagnostics(ctx, errs)

	return nil
}

// DidChange handles a textDocument/didChange notification. The client sends
// this whenever the content of an open document changes, providing either a
// full replacement of the document text or an incremental list of edits,
// depending on what was agreed during capability negotiation.
func (s *zkcServer) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}

	// Use the last change; in full-sync mode this carries the entire updated text.
	text := params.ContentChanges[len(params.ContentChanges)-1].Text
	uri := params.TextDocument.URI

	s.mu.Lock()
	errs := s.compiler.Apply(compiler.ChangedFile(uri.Filename(), text))
	s.mu.Unlock()

	s.publishDiagnostics(ctx, errs)

	return nil
}

// DidClose handles a textDocument/didClose notification. The client sends
// this when a previously-opened document is closed in the editor. After this
// point the server should discard any in-memory state for the document; the
// source of truth reverts to the file system.
func (s *zkcServer) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	filename := params.TextDocument.URI.Filename()

	// On close the source of truth reverts to disk: re-read the saved file
	// so that any unsaved edits the user discarded are dropped from the
	// compiler's view.  If the file has been deleted from disk, evict it.
	var update compiler.FileUpdate
	if contents, err := os.ReadFile(filename); err == nil {
		update = compiler.ChangedFile(filename, string(contents))
	} else {
		update = compiler.RemovedFile(filename)
	}

	s.mu.Lock()
	errs := s.compiler.Apply(update)
	s.mu.Unlock()

	s.publishDiagnostics(ctx, errs)

	return nil
}

// DidSave handles a textDocument/didSave notification. The client sends this
// when the user saves a document, optionally including the full text of the
// document at the point of save. Servers may use this to trigger actions such
// as re-validation or indexing.
func (s *zkcServer) DidSave(_ context.Context, _ *protocol.DidSaveTextDocumentParams) error {
	return nil
}

// WillSave handles a textDocument/willSave notification. The client sends
// this just before a document is saved, providing the reason for the save
// (manual, auto-save, or save-on-focus-loss). Unlike WillSaveWaitUntil, no
// response is expected and the server cannot influence the saved content.
func (s *zkcServer) WillSave(_ context.Context, _ *protocol.WillSaveTextDocumentParams) error {
	return nil
}

// DidChangeConfiguration handles a workspace/didChangeConfiguration
// notification. The client sends this when the user changes settings that are
// relevant to language servers. The notification may carry the new settings
// values, or the server may need to re-fetch them via workspace/configuration.
func (s *zkcServer) DidChangeConfiguration(
	_ context.Context, _ *protocol.DidChangeConfigurationParams,
) error {
	return nil
}

// DidChangeWatchedFiles handles a workspace/didChangeWatchedFiles
// notification. The client sends this when files on disk that the server
// registered interest in are created, modified, or deleted. Servers use this
// to stay consistent with the file system without polling.
func (s *zkcServer) DidChangeWatchedFiles(
	_ context.Context, _ *protocol.DidChangeWatchedFilesParams,
) error {
	return nil
}

// DidChangeWorkspaceFolders handles a workspace/didChangeWorkspaceFolders
// notification. The client sends this when the set of root folders in a
// multi-root workspace changes (folders added or removed). Servers that
// maintain per-folder state should update their internal bookkeeping accordingly.
func (s *zkcServer) DidChangeWorkspaceFolders(
	_ context.Context, _ *protocol.DidChangeWorkspaceFoldersParams,
) error {
	return nil
}

// DidCreateFiles handles a workspace/didCreateFiles notification. The client
// sends this after files or folders have been created, whether by the editor
// UI or via a workspace edit. The server may use this to update indices or
// other derived state that depends on the set of files in the workspace.
func (s *zkcServer) DidCreateFiles(_ context.Context, _ *protocol.CreateFilesParams) error {
	return nil
}

// DidRenameFiles handles a workspace/didRenameFiles notification. The client
// sends this after files or folders have been renamed. The server may use
// this to update cross-file references or other state that encodes file paths.
func (s *zkcServer) DidRenameFiles(_ context.Context, _ *protocol.RenameFilesParams) error {
	return nil
}

// DidDeleteFiles handles a workspace/didDeleteFiles notification. The client
// sends this after files or folders have been deleted. The server may use
// this to evict cached data and remove references to the deleted resources.
func (s *zkcServer) DidDeleteFiles(_ context.Context, _ *protocol.DeleteFilesParams) error {
	return nil
}

// CodeLensRefresh handles a workspace/codeLens/refresh request sent by the
// server to ask the client to invalidate and re-request all code lenses. The
// server issues this when its internal state changes in a way that would
// affect previously-computed lenses across multiple documents.
func (s *zkcServer) CodeLensRefresh(_ context.Context) error { return nil }

// SemanticTokensRefresh handles a workspace/semanticTokens/refresh request
// sent by the server to ask the client to discard cached semantic-token data
// and re-request tokens for any open documents. The server issues this when
// global state (e.g. a type database) changes in a way that invalidates
// previously-sent token information.
func (s *zkcServer) SemanticTokensRefresh(_ context.Context) error { return nil }

// ============================================================================
// Unimplemented requests
// ============================================================================

// CodeAction handles a textDocument/codeAction request. The client sends this
// when the user invokes the "quick fix" or "refactor" UI at a given document
// range, or when the editor is displaying diagnostics and wants to offer
// automated fixes. The server returns a list of commands or workspace edits
// that can be applied to resolve the issue or improve the code.
// Not yet implemented.
func (s *zkcServer) CodeAction(
	_ context.Context, _ *protocol.CodeActionParams,
) ([]protocol.CodeAction, error) {
	return nil, errNotImplemented
}

// CodeLens handles a textDocument/codeLens request. The client sends this to
// retrieve a list of code lenses for a document — small, actionable annotations
// rendered inline above or beside code (e.g. "Run test", "3 references").
// Each lens may carry a command immediately or defer resolution to a
// subsequent codeLens/resolve call.
// Not yet implemented.
func (s *zkcServer) CodeLens(
	_ context.Context, _ *protocol.CodeLensParams,
) ([]protocol.CodeLens, error) {
	return nil, errNotImplemented
}

// CodeLensResolve handles a codeLens/resolve request. When a server returns
// code lenses without a command (deferring to save compute), the client calls
// this to fill in the command for a specific lens just before it is displayed.
// Not yet implemented.
func (s *zkcServer) CodeLensResolve(
	_ context.Context, _ *protocol.CodeLens,
) (*protocol.CodeLens, error) {
	return nil, errNotImplemented
}

// ColorPresentation handles a textDocument/colorPresentation request. After
// a documentColor request identifies a color value in the source, the client
// calls this to get a list of textual representations for that color (e.g.
// "#ff0000", "rgb(255,0,0)") so the user can choose how to rewrite it.
// Not yet implemented.
func (s *zkcServer) ColorPresentation(
	_ context.Context, _ *protocol.ColorPresentationParams,
) ([]protocol.ColorPresentation, error) {
	return nil, errNotImplemented
}

// Completion handles a textDocument/completion request. The client sends this
// when the user triggers auto-complete (e.g. by typing a character or pressing
// a shortcut). The server returns a list of completion items — identifiers,
// keywords, snippets, etc. — that are valid at the cursor position, which the
// editor presents in a pick list.
// Not yet implemented.
func (s *zkcServer) Completion(
	_ context.Context, _ *protocol.CompletionParams,
) (*protocol.CompletionList, error) {
	return nil, errNotImplemented
}

// CompletionResolve handles a completionItem/resolve request. Servers may
// return completion items without expensive fields (e.g. documentation,
// additional edits) in the initial Completion response and fill them in lazily
// here when the user selects a specific item in the pick list.
// Not yet implemented.
func (s *zkcServer) CompletionResolve(
	_ context.Context, _ *protocol.CompletionItem,
) (*protocol.CompletionItem, error) {
	return nil, errNotImplemented
}

// Declaration handles a textDocument/declaration request. The client sends
// this when the user invokes "go to declaration" on a symbol. In languages
// that distinguish declaration from definition (e.g. C/C++ header vs.
// implementation), the server returns the location of the declaration; in
// others it may coincide with the definition.
// Not yet implemented.
func (s *zkcServer) Declaration(
	_ context.Context, _ *protocol.DeclarationParams,
) ([]protocol.Location, error) {
	return nil, errNotImplemented
}

// Definition handles a textDocument/definition request. The client sends this
// when the user invokes "go to definition" on a symbol. The server returns
// the location (file and range) where the symbol is defined, allowing the
// editor to navigate there.
func (s *zkcServer) Definition(
	_ context.Context, params *protocol.DefinitionParams,
) ([]protocol.Location, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.DefinitionFor(params.TextDocument.URI, text, params.Position, program, srcmaps)
}

// DocumentColor handles a textDocument/documentColor request. The client
// sends this to find all color references in a document (e.g. CSS hex codes,
// rgb() literals). The server returns a list of ranges and their parsed color
// values, which the editor uses to render color swatches inline.
// Not yet implemented.
func (s *zkcServer) DocumentColor(
	_ context.Context, _ *protocol.DocumentColorParams,
) ([]protocol.ColorInformation, error) {
	return nil, errNotImplemented
}

// DocumentHighlight handles a textDocument/documentHighlight request. The
// client sends this when the cursor rests on a symbol and wants all other
// occurrences of that symbol in the same document highlighted (e.g. read
// accesses in one shade, write accesses in another).
// Not yet implemented.
func (s *zkcServer) DocumentHighlight(
	_ context.Context, _ *protocol.DocumentHighlightParams,
) ([]protocol.DocumentHighlight, error) {
	return nil, errNotImplemented
}

// DocumentLink handles a textDocument/documentLink request. The client sends
// this to discover all hyperlink-like ranges in a document — for example,
// URLs in comments or import paths — so they can be rendered as clickable
// links or tooltipped.
// Not yet implemented.
func (s *zkcServer) DocumentLink(
	_ context.Context, _ *protocol.DocumentLinkParams,
) ([]protocol.DocumentLink, error) {
	return nil, errNotImplemented
}

// DocumentLinkResolve handles a documentLink/resolve request. When a server
// returns document links without a target URI (deferring to save compute),
// the client calls this to resolve the target for a specific link before it
// is navigated to.
// Not yet implemented.
func (s *zkcServer) DocumentLinkResolve(
	_ context.Context, _ *protocol.DocumentLink,
) (*protocol.DocumentLink, error) {
	return nil, errNotImplemented
}

// DocumentSymbol handles a textDocument/documentSymbol request. The client
// sends this to populate the editor's outline panel or breadcrumb bar. The
// server returns a hierarchical or flat list of symbols (functions, types,
// constants, etc.) present in the document, each with a name, kind, and range.
func (s *zkcServer) DocumentSymbol(
	_ context.Context, params *protocol.DocumentSymbolParams,
) ([]interface{}, error) {
	s.mu.RLock()
	_, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.DocumentSymbolsFor(params.TextDocument.URI, program, srcmaps)
}

// ExecuteCommand handles a workspace/executeCommand request. Servers
// advertise a list of commands in their capabilities; the client invokes this
// when the user triggers one (e.g. via a code action or code lens). The server
// carries out the command, typically by applying a workspace edit or
// performing some side effect, and optionally returns a result.
// Not yet implemented.
func (s *zkcServer) ExecuteCommand(
	_ context.Context, _ *protocol.ExecuteCommandParams,
) (interface{}, error) {
	return nil, errNotImplemented
}

// FoldingRanges handles a textDocument/foldingRange request. The client sends
// this to learn which regions of a document can be collapsed in the editor
// (e.g. function bodies, block comments, import groups). The server returns a
// list of ranges and optional kind hints (comment, imports, region).
// Not yet implemented.
func (s *zkcServer) FoldingRanges(
	_ context.Context, _ *protocol.FoldingRangeParams,
) ([]protocol.FoldingRange, error) {
	return nil, errNotImplemented
}

// Formatting handles a textDocument/formatting request. The client sends this
// when the user invokes "format document". The server returns a list of text
// edits that, when applied, produce a fully formatted version of the document
// according to the language's style rules.
func (s *zkcServer) Formatting(
	_ context.Context, params *protocol.DocumentFormattingParams,
) ([]protocol.TextEdit, error) {
	uri := params.TextDocument.URI

	s.mu.RLock()
	text, ok := s.compiler.Source(uri.Filename())
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.FormattingFor(uri, text)
}

// Hover handles a textDocument/hover request. The client sends this when the
// cursor rests on a token and requests contextual information to display in a
// popup — typically the type signature of a symbol, its documentation comment,
// or an evaluated value.
func (s *zkcServer) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.HoverFor(params.TextDocument.URI, text, params.Position, program, srcmaps)
}

// Implementation handles a textDocument/implementation request. The client
// sends this when the user invokes "go to implementation" on an interface,
// abstract method, or similar construct. The server returns the locations of
// the concrete types or methods that satisfy the abstraction.
// Not yet implemented.
func (s *zkcServer) Implementation(
	_ context.Context, _ *protocol.ImplementationParams,
) ([]protocol.Location, error) {
	return nil, errNotImplemented
}

// OnTypeFormatting handles a textDocument/onTypeFormatting request. The client
// sends this as the user types certain trigger characters (e.g. "}", ";") that
// should cause the server to return formatting edits applied immediately,
// without requiring the user to explicitly invoke format.
// Not yet implemented.
func (s *zkcServer) OnTypeFormatting(
	_ context.Context, _ *protocol.DocumentOnTypeFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, errNotImplemented
}

// PrepareRename handles a textDocument/prepareRename request. Before
// committing to a rename operation the client calls this to ask whether a
// rename is valid at the given position and, if so, what range should be
// pre-selected in the rename input box. The server may reject the request if
// the symbol cannot be renamed (e.g. it is a built-in).
func (s *zkcServer) PrepareRename(
	_ context.Context, params *protocol.PrepareRenameParams,
) (*protocol.Range, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.PrepareRenameFor(params.TextDocument.URI, text, params.Position, program, srcmaps)
}

// RangeFormatting handles a textDocument/rangeFormatting request. The client
// sends this when the user selects a region of text and invokes "format
// selection". The server returns a list of text edits covering only the
// selected range, leaving the rest of the document unchanged.
// Not yet implemented.
func (s *zkcServer) RangeFormatting(
	_ context.Context, _ *protocol.DocumentRangeFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, errNotImplemented
}

// References handles a textDocument/references request. The client sends this
// when the user invokes "find all references" on a symbol. The server returns
// every location in the workspace where that symbol is used, optionally
// including the declaration site itself.
func (s *zkcServer) References(
	_ context.Context, params *protocol.ReferenceParams,
) ([]protocol.Location, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.ReferencesFor(
		params.TextDocument.URI, text, params.Position,
		params.Context.IncludeDeclaration, program, srcmaps,
	)
}

// Rename handles a textDocument/rename request. The client sends this when
// the user confirms a rename operation (following an optional PrepareRename).
// The server returns a workspace edit — a set of text changes across all
// affected files — that renames every occurrence of the symbol.
func (s *zkcServer) Rename(
	_ context.Context, params *protocol.RenameParams,
) (*protocol.WorkspaceEdit, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	srcmaps := s.compiler.SourceMaps()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.RenameFor(
		params.TextDocument.URI, text, params.Position, params.NewName,
		program, srcmaps,
	)
}

// SignatureHelp handles a textDocument/signatureHelp request. The client
// sends this while the user is typing arguments inside a function call, to
// display the callee's parameter list as a tooltip. The server returns the
// matching overloads and identifies which parameter is active at the cursor.
func (s *zkcServer) SignatureHelp(
	_ context.Context, params *protocol.SignatureHelpParams,
) (*protocol.SignatureHelp, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	program := s.compiler.Program()
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.SignatureHelpFor(params.TextDocument.URI, text, params.Position, program)
}

// Symbols handles a workspace/symbol request. The client sends this when the
// user opens the "go to symbol in workspace" picker and types a query string.
// The server returns all matching symbols across the entire workspace, not
// just the open document, enabling fast cross-file navigation.
// Not yet implemented.
func (s *zkcServer) Symbols(
	_ context.Context, _ *protocol.WorkspaceSymbolParams,
) ([]protocol.SymbolInformation, error) {
	return nil, errNotImplemented
}

// TypeDefinition handles a textDocument/typeDefinition request. The client
// sends this when the user invokes "go to type definition" on a variable or
// expression. The server returns the location where the type of that symbol
// is defined, which may differ from where the variable itself is declared.
// Not yet implemented.
func (s *zkcServer) TypeDefinition(
	_ context.Context, _ *protocol.TypeDefinitionParams,
) ([]protocol.Location, error) {
	return nil, errNotImplemented
}

// WillSaveWaitUntil handles a textDocument/willSaveWaitUntil request. Unlike
// the WillSave notification, the client awaits a response before writing the
// file. The server may return text edits (e.g. removing trailing whitespace
// or inserting an auto-import) that are applied to the document immediately
// before it is saved to disk.
// Not yet implemented.
func (s *zkcServer) WillSaveWaitUntil(
	_ context.Context, _ *protocol.WillSaveTextDocumentParams,
) ([]protocol.TextEdit, error) {
	return nil, errNotImplemented
}

// ShowDocument handles a window/showDocument request sent by the server to
// ask the client to display a particular URI (which may be an external URL or
// a file in the workspace) in the editor, optionally scrolling to a specific
// range and taking focus.
// Not yet implemented.
func (s *zkcServer) ShowDocument(
	_ context.Context, _ *protocol.ShowDocumentParams,
) (*protocol.ShowDocumentResult, error) {
	return nil, errNotImplemented
}

// WillCreateFiles handles a workspace/willCreateFiles request. Before files
// are created (e.g. via a "new file" action initiated by the editor), the
// client calls this so the server can return workspace edits that should be
// applied as part of the creation — for example, inserting a package
// declaration or licence header.
// Not yet implemented.
func (s *zkcServer) WillCreateFiles(
	_ context.Context, _ *protocol.CreateFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, errNotImplemented
}

// WillRenameFiles handles a workspace/willRenameFiles request. Before files
// are renamed the client calls this so the server can return workspace edits
// that keep the codebase consistent — for example, updating import paths or
// other references that encode the old file name.
// Not yet implemented.
func (s *zkcServer) WillRenameFiles(
	_ context.Context, _ *protocol.RenameFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, errNotImplemented
}

// WillDeleteFiles handles a workspace/willDeleteFiles request. Before files
// are deleted the client calls this so the server can return workspace edits
// that clean up references to the files being removed, such as import
// statements or symbol declarations that would become dangling.
// Not yet implemented.
func (s *zkcServer) WillDeleteFiles(
	_ context.Context, _ *protocol.DeleteFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, errNotImplemented
}

// PrepareCallHierarchy handles a textDocument/prepareCallHierarchy request.
// The client sends this when the user invokes the call hierarchy UI on a
// symbol. The server returns one or more CallHierarchyItem values that
// identify the symbol and serve as anchors for subsequent incomingCalls and
// outgoingCalls queries.
// Not yet implemented.
func (s *zkcServer) PrepareCallHierarchy(
	_ context.Context, _ *protocol.CallHierarchyPrepareParams,
) ([]protocol.CallHierarchyItem, error) {
	return nil, errNotImplemented
}

// IncomingCalls handles a callHierarchy/incomingCalls request. Given a
// CallHierarchyItem returned by PrepareCallHierarchy, the server returns all
// the sites in the workspace that call into that item, enabling the user to
// trace who calls a given function.
// Not yet implemented.
func (s *zkcServer) IncomingCalls(
	_ context.Context, _ *protocol.CallHierarchyIncomingCallsParams,
) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, errNotImplemented
}

// OutgoingCalls handles a callHierarchy/outgoingCalls request. Given a
// CallHierarchyItem, the server returns all calls made from within that item
// to other functions, enabling the user to trace what a given function calls.
// Not yet implemented.
func (s *zkcServer) OutgoingCalls(
	_ context.Context, _ *protocol.CallHierarchyOutgoingCallsParams,
) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, errNotImplemented
}

// SemanticTokensFull handles a textDocument/semanticTokens/full request. The
// client sends this to obtain semantic highlighting data for an entire
// document. Rather than relying on syntactic tokenisation, the server
// classifies tokens by their semantic role (e.g. type name, variable, keyword)
// so the editor can apply richer, more accurate syntax colouring.
func (s *zkcServer) SemanticTokensFull(
	_ context.Context, params *protocol.SemanticTokensParams,
) (*protocol.SemanticTokens, error) {
	s.mu.RLock()
	text, ok := s.compiler.Source(params.TextDocument.URI.Filename())
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return lsp.SemanticTokensFor(params.TextDocument.URI, text)
}

// SemanticTokensFullDelta handles a textDocument/semanticTokens/full/delta
// request. Instead of re-sending the entire token set after each edit, the
// client sends this to request only the changes (insertions, deletions,
// replacements) relative to the token set returned by the previous full or
// delta response, identified by the result ID.
// Not yet implemented.
func (s *zkcServer) SemanticTokensFullDelta(
	_ context.Context, _ *protocol.SemanticTokensDeltaParams,
) (interface{}, error) {
	return nil, errNotImplemented
}

// SemanticTokensRange handles a textDocument/semanticTokens/range request.
// The client sends this to fetch semantic tokens for a sub-range of a
// document (e.g. the visible viewport), rather than the whole file. This
// allows incremental loading of token data for large documents.
// Not yet implemented.
func (s *zkcServer) SemanticTokensRange(
	_ context.Context, _ *protocol.SemanticTokensRangeParams,
) (*protocol.SemanticTokens, error) {
	return nil, errNotImplemented
}

// LinkedEditingRange handles a textDocument/linkedEditingRange request. The
// client sends this when the cursor is on a symbol that has a paired
// counterpart that should be renamed simultaneously — the canonical example
// being matching HTML/XML open and close tags. The server returns the ranges
// that must be kept in sync.
// Not yet implemented.
func (s *zkcServer) LinkedEditingRange(
	_ context.Context, _ *protocol.LinkedEditingRangeParams,
) (*protocol.LinkedEditingRanges, error) {
	return nil, errNotImplemented
}

// Moniker handles a textDocument/moniker request. A moniker is a
// language-independent, globally unique identifier for a symbol, used to
// correlate symbols across repository and package boundaries (e.g. for
// cross-repository "find references" in code intelligence platforms). The
// server returns the monikers associated with the symbol at the given position.
// Not yet implemented.
func (s *zkcServer) Moniker(
	_ context.Context, _ *protocol.MonikerParams,
) ([]protocol.Moniker, error) {
	return nil, errNotImplemented
}

// Request handles any LSP method that is not covered by the typed dispatch in
// ServerHandler — typically non-standard or future protocol extensions. The
// method name is passed as a string so the server can decide how to respond;
// unrecognised methods return a method-not-found error.
func (s *zkcServer) Request(_ context.Context, method string, _ interface{}) (interface{}, error) {
	return nil, fmt.Errorf("%q: %w", method, jsonrpc2.ErrMethodNotFound)
}

//nolint:errcheck
func init() {
	lspCmd.Flags().BoolVarP(&lspVerbose, "verbose", "v", false, "increase logging verbosity")
	lspCmd.Flags().Uint16VarP(&lspPort, "port", "p", 0, "TCP port to listen on (default 0: use stdio)")
	lspCmd.Flags().StringVarP(&lspLog, "log", "l", "", "write log output to this file (default: no logging)")
	rootCmd.AddCommand(lspCmd)
}
