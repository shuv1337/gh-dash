package section

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/go-sprout/sprout"
	timeregistry "github.com/go-sprout/sprout/registry/time"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/git"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prompt"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/repopicker"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/search"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

// FilterTarget represents which repository to filter by when smart filtering is enabled
type FilterTarget int

const (
	FilterTargetOrigin   FilterTarget = iota // Filter by origin (current fork)
	FilterTargetUpstream                     // Filter by upstream (parent repo)
	FilterTargetNone                         // No repo filter applied
)

type BaseModel struct {
	Id                        int
	Config                    config.SectionConfig
	Ctx                       *context.ProgramContext
	Spinner                   spinner.Model
	SearchBar                 search.Model
	IsSearching               bool
	SearchValue               string
	Table                     table.Model
	Type                      string
	SingularForm              string
	PluralForm                string
	Columns                   []table.Column
	TotalCount                int
	PageInfo                  *data.PageInfo
	PromptConfirmationBox     prompt.Model
	IsPromptConfirmationShown bool
	PromptConfirmationAction  string
	LastFetchTaskId           string
	IsSearchSupported         bool
	ShowAuthorIcon            bool
	IsFilteredByCurrentRemote bool
	IsLoading                 bool
	// FilterTarget indicates which repo to filter by (origin, upstream, or none)
	FilterTarget FilterTarget
	// IsAuthorFilterRemoved indicates if the author:@me filter has been removed
	IsAuthorFilterRemoved bool
	// CustomRepoFilter is a manually specified repo filter that overrides FilterTarget
	CustomRepoFilter string
	// IsRepoPickerShown indicates if the repo picker is currently shown
	IsRepoPickerShown bool
	// RepoPicker is the repo picker component
	RepoPicker repopicker.Model
}

type NewSectionOptions struct {
	Id          int
	Config      config.SectionConfig
	Ctx         *context.ProgramContext
	Type        string
	Columns     []table.Column
	Singular    string
	Plural      string
	LastUpdated time.Time
	CreatedAt   time.Time
}

func (options NewSectionOptions) GetConfigFiltersWithCurrentRemoteAdded(ctx *context.ProgramContext) string {
	searchValue := options.Config.Filters
	if !ctx.Config.SmartFilteringAtLaunch {
		return searchValue
	}

	// Get origin from git remote directly, not repository.Current()
	// which may resolve to the upstream/parent repo instead of the fork
	repoDir := "."
	if ctx != nil && ctx.RepoPath != "" {
		repoDir = ctx.RepoPath
	}
	originUrl, err := git.GetOriginUrl(repoDir)
	if err != nil {
		return searchValue
	}
	owner, name, err := git.ParseGitHubRepoFromUrl(originUrl)
	if err != nil {
		return searchValue
	}

	for token := range strings.FieldsSeq(searchValue) {
		if strings.HasPrefix(token, "repo:") {
			return searchValue
		}
	}
	return fmt.Sprintf("repo:%s/%s %s", owner, name, searchValue)
}

func NewModel(
	ctx *context.ProgramContext,
	options NewSectionOptions,
) BaseModel {
	filters := options.GetConfigFiltersWithCurrentRemoteAdded(ctx)
	filterTarget := FilterTargetNone
	if ctx.Config.SmartFilteringAtLaunch && filters != options.Config.Filters {
		filterTarget = FilterTargetOrigin
	}
	m := BaseModel{
		Ctx:          ctx,
		Id:           options.Id,
		Type:         options.Type,
		Config:       options.Config,
		Spinner:      spinner.Model{Spinner: spinner.Dot},
		Columns:      options.Columns,
		SingularForm: options.Singular,
		PluralForm:   options.Plural,
		SearchBar: search.NewModel(ctx, search.SearchOptions{
			Prefix:       fmt.Sprintf("is:%s", options.Type),
			InitialValue: filters,
		}),
		SearchValue:               filters,
		IsSearching:               false,
		IsFilteredByCurrentRemote: filters != options.Config.Filters,
		TotalCount:                0,
		PageInfo:                  nil,
		PromptConfirmationBox:     prompt.NewModel(ctx),
		ShowAuthorIcon:            ctx.Config.ShowAuthorIcons,
		FilterTarget:              filterTarget,
		IsAuthorFilterRemoved:     false,
		CustomRepoFilter:          "",
		IsRepoPickerShown:         false,
		RepoPicker:                repopicker.NewModel(ctx),
	}
	if !ctx.Config.SmartFilteringAtLaunch {
		m.IsFilteredByCurrentRemote = false
		m.FilterTarget = FilterTargetNone
	}
	m.Table = table.NewModel(
		*ctx,
		m.GetDimensions(),
		options.LastUpdated,
		options.CreatedAt,
		m.Columns,
		nil,
		m.SingularForm,
		utils.StringPtr(m.Ctx.Styles.Section.EmptyStateStyle.Render(
			fmt.Sprintf(
				"No %s were found that match the given filters",
				m.PluralForm,
			),
		)),
		"Loading...",
		false,
	)
	return m
}

type Section interface {
	Identifier
	Component
	Table
	Search
	PromptConfirmation
	GetConfig() config.SectionConfig
	UpdateProgramContext(ctx *context.ProgramContext)
	MakeSectionCmd(cmd tea.Cmd) tea.Cmd
	GetPagerContent() string
	GetItemSingularForm() string
	GetItemPluralForm() string
	GetTotalCount() int
}

type Identifier interface {
	GetId() int
	GetType() string
}

type Component interface {
	Update(msg tea.Msg) (Section, tea.Cmd)
	View() string
}

type Table interface {
	NumRows() int
	GetCurrRow() data.RowData
	CurrRow() int
	NextRow() int
	PrevRow() int
	FirstItem() int
	LastItem() int
	FetchNextPageSectionRows() []tea.Cmd
	BuildRows() []table.Row
	ResetRows()
	GetIsLoading() bool
	SetIsLoading(val bool)
}

type Search interface {
	SetIsSearching(val bool) tea.Cmd
	IsSearchFocused() bool
	ResetFilters()
	GetFilters() string
	ResetPageInfo()
	IsFilteringByClone() bool
}

type PromptConfirmation interface {
	SetIsPromptConfirmationShown(val bool) tea.Cmd
	IsPromptConfirmationFocused() bool
	SetPromptConfirmationAction(action string)
	GetPromptConfirmationAction() string
	GetPromptConfirmation() string
}

func (m *BaseModel) GetDimensions() constants.Dimensions {
	return constants.Dimensions{
		Width:  max(0, m.Ctx.MainContentWidth-m.Ctx.Styles.Section.ContainerStyle.GetHorizontalPadding()),
		Height: max(0, m.Ctx.MainContentHeight-common.SearchHeight),
	}
}

func (m *BaseModel) GetConfig() config.SectionConfig {
	return m.Config
}

func (m *BaseModel) HasRepoNameInConfiguredFilter() bool {
	filters := m.Config.Filters
	for token := range strings.FieldsSeq(filters) {
		if strings.HasPrefix(token, "repo:") {
			return true
		}
	}
	return false
}

func (m *BaseModel) GetSearchValue() string {
	searchValue := m.enrichSearchWithTemplateVars()

	// If there's a custom repo filter set via the picker, use it
	if m.CustomRepoFilter != "" {
		return m.applyRepoFilter(searchValue, m.CustomRepoFilter)
	}

	// Get origin repo from git remote (not repository.Current() which may resolve to upstream)
	originOwner, originName, hasOrigin := m.GetOriginRepo()
	if !hasOrigin {
		return m.applyAuthorFilter(searchValue)
	}

	if m.HasRepoNameInConfiguredFilter() {
		return m.applyAuthorFilter(searchValue)
	}

	// Check if the user manually added a repo: filter in the search
	if m.hasManualRepoFilter(searchValue) {
		// User has manually specified a repo filter, respect it
		return m.applyAuthorFilter(searchValue)
	}

	// Remove any existing repo filters from the search value
	var tokensWithoutRepoFilter []string
	for token := range strings.FieldsSeq(searchValue) {
		if !strings.HasPrefix(token, "repo:") {
			tokensWithoutRepoFilter = append(tokensWithoutRepoFilter, token)
		}
	}
	searchValueWithoutRepoFilter := strings.Join(tokensWithoutRepoFilter, " ")

	// Apply the appropriate repo filter based on FilterTarget
	var result string
	switch m.FilterTarget {
	case FilterTargetOrigin:
		result = fmt.Sprintf("repo:%s/%s %s", originOwner, originName, searchValueWithoutRepoFilter)
	case FilterTargetUpstream:
		upstreamOwner, upstreamName, hasUpstream := m.GetUpstreamRepo()
		if hasUpstream {
			result = fmt.Sprintf("repo:%s/%s %s", upstreamOwner, upstreamName, searchValueWithoutRepoFilter)
		} else {
			// No upstream found, fall back to origin
			result = fmt.Sprintf("repo:%s/%s %s", originOwner, originName, searchValueWithoutRepoFilter)
		}
	default:
		result = searchValueWithoutRepoFilter
	}

	return m.applyAuthorFilter(result)
}

// hasManualRepoFilter checks if the search value contains a manually-entered repo filter
// that differs from what would be auto-applied by FilterTarget
func (m *BaseModel) hasManualRepoFilter(searchValue string) bool {
	for token := range strings.FieldsSeq(searchValue) {
		if strings.HasPrefix(token, "repo:") {
			// There's a repo filter - check if it matches our auto-applied filters
			repoValue := strings.TrimPrefix(token, "repo:")

			// Get origin from git remote (not repository.Current())
			originOwner, originName, hasOrigin := m.GetOriginRepo()
			if !hasOrigin {
				return true // Can't determine, assume it's manual
			}

			originRepo := fmt.Sprintf("%s/%s", originOwner, originName)
			if repoValue == originRepo && m.FilterTarget == FilterTargetOrigin {
				return false // Matches auto-applied origin
			}

			upstreamOwner, upstreamName, hasUpstream := m.GetUpstreamRepo()
			if hasUpstream {
				upstreamRepo := fmt.Sprintf("%s/%s", upstreamOwner, upstreamName)
				if repoValue == upstreamRepo && m.FilterTarget == FilterTargetUpstream {
					return false // Matches auto-applied upstream
				}
			}

			// It's a different repo filter, treat as manual
			return true
		}
	}
	return false
}

// StripRepoFilterTokens removes any repo:... tokens from a search string.
func StripRepoFilterTokens(searchValue string) string {
	var tokensWithoutRepoFilter []string
	for token := range strings.FieldsSeq(searchValue) {
		if !strings.HasPrefix(token, "repo:") {
			tokensWithoutRepoFilter = append(tokensWithoutRepoFilter, token)
		}
	}
	return strings.Join(tokensWithoutRepoFilter, " ")
}

func getRepoFilterTokenValue(searchValue string) (string, bool) {
	for token := range strings.FieldsSeq(searchValue) {
		if strings.HasPrefix(token, "repo:") {
			return strings.TrimPrefix(token, "repo:"), true
		}
	}
	return "", false
}

// applyRepoFilter applies a specific repo filter to the search value
func (m *BaseModel) applyRepoFilter(searchValue, repoFilter string) string {
	// Remove any existing repo filters
	var tokensWithoutRepoFilter []string
	for token := range strings.FieldsSeq(searchValue) {
		if !strings.HasPrefix(token, "repo:") {
			tokensWithoutRepoFilter = append(tokensWithoutRepoFilter, token)
		}
	}
	searchValueWithoutRepoFilter := strings.Join(tokensWithoutRepoFilter, " ")

	var result string
	switch {
	case repoFilter == "":
		result = searchValueWithoutRepoFilter
	case searchValueWithoutRepoFilter == "":
		result = fmt.Sprintf("repo:%s", repoFilter)
	default:
		result = fmt.Sprintf("repo:%s %s", repoFilter, searchValueWithoutRepoFilter)
	}

	return m.applyAuthorFilter(result)
}

// applyAuthorFilter removes author:@me from the search if IsAuthorFilterRemoved is true
func (m *BaseModel) applyAuthorFilter(searchValue string) string {
	if !m.IsAuthorFilterRemoved {
		return searchValue
	}

	var tokensWithoutAuthor []string
	for token := range strings.FieldsSeq(searchValue) {
		if token != "author:@me" {
			tokensWithoutAuthor = append(tokensWithoutAuthor, token)
		}
	}
	return strings.Join(tokensWithoutAuthor, " ")
}

// getRepoDir returns the directory to use for git operations.
// Uses m.Ctx.RepoPath if available, otherwise falls back to ".".
func (m *BaseModel) getRepoDir() string {
	if m.Ctx != nil && m.Ctx.RepoPath != "" {
		return m.Ctx.RepoPath
	}
	return "."
}

// GetOriginRepo returns the owner and name of the origin repository
func (m *BaseModel) GetOriginRepo() (owner, name string, hasOrigin bool) {
	originUrl, err := git.GetOriginUrl(m.getRepoDir())
	if err != nil {
		return "", "", false
	}
	owner, name, err = git.ParseGitHubRepoFromUrl(originUrl)
	if err != nil {
		return "", "", false
	}
	return owner, name, true
}

// GetUpstreamRepo returns the owner and name of the upstream repository, if available
func (m *BaseModel) GetUpstreamRepo() (owner, name string, hasUpstream bool) {
	upstreamUrl, err := git.GetUpstreamUrl(m.getRepoDir())
	if err != nil {
		return "", "", false
	}
	owner, name, err = git.ParseGitHubRepoFromUrl(upstreamUrl)
	if err != nil {
		return "", "", false
	}
	return owner, name, true
}

// HasUpstreamRemote returns true if an upstream remote is configured
func (m *BaseModel) HasUpstreamRemote() bool {
	_, _, hasUpstream := m.GetUpstreamRepo()
	return hasUpstream
}

// ToggleFilterTarget cycles through filter targets: Origin -> Upstream -> None -> Origin
func (m *BaseModel) ToggleFilterTarget() {
	if m.HasRepoNameInConfiguredFilter() {
		return // Don't toggle if repo is explicitly set in config
	}

	hasUpstream := m.HasUpstreamRemote()

	switch m.FilterTarget {
	case FilterTargetOrigin:
		if hasUpstream {
			m.FilterTarget = FilterTargetUpstream
		} else {
			m.FilterTarget = FilterTargetNone
		}
	case FilterTargetUpstream:
		m.FilterTarget = FilterTargetNone
	case FilterTargetNone:
		m.FilterTarget = FilterTargetOrigin
	}

	// Keep IsFilteredByCurrentRemote in sync for backward compatibility
	m.IsFilteredByCurrentRemote = m.FilterTarget != FilterTargetNone
}

// ToggleAuthorFilter toggles whether the author:@me filter is removed
func (m *BaseModel) ToggleAuthorFilter() {
	m.IsAuthorFilterRemoved = !m.IsAuthorFilterRemoved
}

// ShowRepoPicker shows the repo picker with available options
func (m *BaseModel) ShowRepoPicker() tea.Cmd {
	options := m.buildRepoPickerOptions()
	m.RepoPicker.SetOptions(options)
	m.RepoPicker.SetWidth(50)

	// Set the current selection based on the search query (source of truth)
	currentRepo, _ := getRepoFilterTokenValue(m.SearchValue)
	m.RepoPicker.SetSelectedValue(currentRepo)

	m.RepoPicker.Focus()
	m.IsRepoPickerShown = true
	return nil
}

// HideRepoPicker hides the repo picker
func (m *BaseModel) HideRepoPicker() {
	m.RepoPicker.Blur()
	m.IsRepoPickerShown = false
}

// IsRepoPickerFocused returns true if the repo picker is focused
func (m *BaseModel) IsRepoPickerFocused() bool {
	return m.IsRepoPickerShown
}

// SetCustomRepoFilter sets a custom repo filter
func (m *BaseModel) SetCustomRepoFilter(repo string) {
	m.CustomRepoFilter = repo
	// Clear the filter target since we're using a custom filter
	if repo != "" {
		m.FilterTarget = FilterTargetNone
		m.IsFilteredByCurrentRemote = true
	}
}

// ClearCustomRepoFilter clears the custom repo filter
func (m *BaseModel) ClearCustomRepoFilter() {
	m.CustomRepoFilter = ""
}

func (m *BaseModel) SyncRepoFilterStateFromSearchValue() {
	repoValue, ok := getRepoFilterTokenValue(m.SearchValue)
	if !ok || repoValue == "" {
		m.CustomRepoFilter = ""
		m.FilterTarget = FilterTargetNone
		m.IsFilteredByCurrentRemote = false
		return
	}

	originOwner, originName, hasOrigin := m.GetOriginRepo()
	if hasOrigin {
		originRepo := fmt.Sprintf("%s/%s", originOwner, originName)
		if repoValue == originRepo {
			m.CustomRepoFilter = ""
			m.FilterTarget = FilterTargetOrigin
			m.IsFilteredByCurrentRemote = true
			return
		}
	}

	upstreamOwner, upstreamName, hasUpstream := m.GetUpstreamRepo()
	if hasUpstream {
		upstreamRepo := fmt.Sprintf("%s/%s", upstreamOwner, upstreamName)
		if repoValue == upstreamRepo {
			m.CustomRepoFilter = ""
			m.FilterTarget = FilterTargetUpstream
			m.IsFilteredByCurrentRemote = true
			return
		}
	}

	m.CustomRepoFilter = repoValue
	m.FilterTarget = FilterTargetNone
	m.IsFilteredByCurrentRemote = true
}

// buildRepoPickerOptions builds the list of repo options for the picker
func (m *BaseModel) buildRepoPickerOptions() []repopicker.RepoOption {
	var options []repopicker.RepoOption

	// Add origin (current fork) - use git remote directly, not repository.Current()
	// which may resolve to the upstream/parent repo
	originOwner, originName, hasOrigin := m.GetOriginRepo()
	if hasOrigin {
		originRepo := fmt.Sprintf("%s/%s", originOwner, originName)
		options = append(options, repopicker.RepoOption{
			Label: fmt.Sprintf("Origin: %s", originRepo),
			Value: originRepo,
			Desc:  "Your fork / current repo",
		})
	}

	// Add upstream if available
	upstreamOwner, upstreamName, hasUpstream := m.GetUpstreamRepo()
	if hasUpstream {
		upstreamRepo := fmt.Sprintf("%s/%s", upstreamOwner, upstreamName)
		options = append(options, repopicker.RepoOption{
			Label: fmt.Sprintf("Upstream: %s", upstreamRepo),
			Value: upstreamRepo,
			Desc:  "Parent repository",
		})
	}

	// Add "All repos" option
	options = append(options, repopicker.RepoOption{
		Label: "All Repositories",
		Value: "",
		Desc:  "No repo filter",
	})

	return options
}

// getCurrentRepoFilter returns the currently active repo filter value
func (m *BaseModel) getCurrentRepoFilter() string {
	if m.CustomRepoFilter != "" {
		return m.CustomRepoFilter
	}

	switch m.FilterTarget {
	case FilterTargetOrigin:
		owner, name, hasOrigin := m.GetOriginRepo()
		if hasOrigin {
			return fmt.Sprintf("%s/%s", owner, name)
		}
	case FilterTargetUpstream:
		owner, name, hasUpstream := m.GetUpstreamRepo()
		if hasUpstream {
			return fmt.Sprintf("%s/%s", owner, name)
		}
	}

	return ""
}

// HandleRepoSelected handles when a repo is selected from the picker
func (m *BaseModel) HandleRepoSelected(value string, isCustom bool) {
	m.HideRepoPicker()

	if value == "" {
		// "All repos" selected
		m.CustomRepoFilter = ""
		m.FilterTarget = FilterTargetNone
		m.IsFilteredByCurrentRemote = false
	} else {
		// Check if value matches origin or upstream
		originOwner, originName, hasOrigin := m.GetOriginRepo()
		if hasOrigin {
			originRepo := fmt.Sprintf("%s/%s", originOwner, originName)
			if value == originRepo {
				m.CustomRepoFilter = ""
				m.FilterTarget = FilterTargetOrigin
				m.IsFilteredByCurrentRemote = true
				return
			}
		}

		upstreamOwner, upstreamName, hasUpstream := m.GetUpstreamRepo()
		if hasUpstream {
			upstreamRepo := fmt.Sprintf("%s/%s", upstreamOwner, upstreamName)
			if value == upstreamRepo {
				m.CustomRepoFilter = ""
				m.FilterTarget = FilterTargetUpstream
				m.IsFilteredByCurrentRemote = true
				return
			}
		}

		// It's a custom repo
		m.CustomRepoFilter = value
		m.FilterTarget = FilterTargetNone
		m.IsFilteredByCurrentRemote = true
	}
}

// GetFilterTargetLabel returns a human-readable label for the current filter target
func (m *BaseModel) GetFilterTargetLabel() string {
	// If there's a custom repo filter, show it
	if m.CustomRepoFilter != "" {
		return m.CustomRepoFilter
	}

	switch m.FilterTarget {
	case FilterTargetOrigin:
		owner, name, hasOrigin := m.GetOriginRepo()
		if hasOrigin {
			return fmt.Sprintf("%s/%s", owner, name)
		}
		return "origin"
	case FilterTargetUpstream:
		owner, name, hasUpstream := m.GetUpstreamRepo()
		if hasUpstream {
			return fmt.Sprintf("%s/%s", owner, name)
		}
		return "upstream"
	default:
		return "all"
	}
}

func (m *BaseModel) enrichSearchWithTemplateVars() string {
	searchValue := m.SearchValue
	searchVars := struct{ Now time.Time }{
		Now: time.Now(),
	}
	sl := slog.New(log.Default())
	handler := sprout.New(sprout.WithRegistries(timeregistry.NewRegistry(), utils.NewRegistry()), sprout.WithLogger(sl))
	funcs := handler.Build()

	tmpl, err := template.New("search").Funcs(funcs).Parse(searchValue)
	if err != nil {
		log.Error("bad template", "err", err)
		return searchValue
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, searchVars)
	if err != nil {
		return searchValue
	}

	return buf.String()
}

func (m *BaseModel) UpdateProgramContext(ctx *context.ProgramContext) {
	m.Ctx = ctx
	newDimensions := m.GetDimensions()
	tableDimensions := constants.Dimensions{
		Height: max(0, newDimensions.Height-2),
		Width:  max(0, newDimensions.Width),
	}
	m.Table.SetDimensions(tableDimensions)
	m.Table.UpdateProgramContext(ctx)
	m.Table.SyncViewPortContent()
	m.SearchBar.UpdateProgramContext(ctx)
	m.RepoPicker.UpdateProgramContext(ctx)
}

type SectionRowsFetchedMsg struct {
	SectionId int
	Issues    []data.RowData
}

func (msg SectionRowsFetchedMsg) GetSectionId() int {
	return msg.SectionId
}

func (m *BaseModel) GetId() int {
	return m.Id
}

func (m *BaseModel) GetType() string {
	return m.Type
}

func (m *BaseModel) CurrRow() int {
	return m.Table.GetCurrItem()
}

func (m *BaseModel) NextRow() int {
	return m.Table.NextItem()
}

func (m *BaseModel) PrevRow() int {
	return m.Table.PrevItem()
}

func (m *BaseModel) FirstItem() int {
	return m.Table.FirstItem()
}

func (m *BaseModel) LastItem() int {
	return m.Table.LastItem()
}

func (m *BaseModel) IsSearchFocused() bool {
	return m.IsSearching
}

func (m *BaseModel) GetIsLoading() bool {
	return m.IsLoading
}

func (m *BaseModel) SetIsSearching(val bool) tea.Cmd {
	m.IsSearching = val
	if val {
		m.SearchBar.Focus()
		return m.SearchBar.Init()
	} else {
		m.SearchBar.Blur()
		return nil
	}
}

func (m *BaseModel) ResetFilters() {
	m.SearchBar.SetValue(m.GetSearchValue())
}

func (m *BaseModel) ResetPageInfo() {
	m.PageInfo = nil
}

func (m *BaseModel) IsPromptConfirmationFocused() bool {
	return m.IsPromptConfirmationShown
}

func (m *BaseModel) SetIsPromptConfirmationShown(val bool) tea.Cmd {
	m.IsPromptConfirmationShown = val
	if val {
		m.PromptConfirmationBox.Focus()
		return m.PromptConfirmationBox.Init()
	}

	m.PromptConfirmationBox.Blur()
	return nil
}

func (m *BaseModel) SetPromptConfirmationAction(action string) {
	m.PromptConfirmationAction = action
}

func (m *BaseModel) GetPromptConfirmationAction() string {
	return m.PromptConfirmationAction
}

type SectionMsg struct {
	Id          int
	Type        string
	InternalMsg tea.Msg
}

func (m *BaseModel) MakeSectionCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		internalMsg := cmd()
		return SectionMsg{
			Id:          m.Id,
			Type:        m.Type,
			InternalMsg: internalMsg,
		}
	}
}

func (m *BaseModel) GetFilters() string {
	return m.GetSearchValue()
}

func (m *BaseModel) IsFilteringByClone() bool {
	return m.IsFilteredByCurrentRemote
}

func (m *BaseModel) GetMainContent() string {
	if m.Table.Rows == nil {
		d := m.GetDimensions()
		return lipgloss.Place(
			d.Width,
			d.Height,
			lipgloss.Center,
			lipgloss.Center,

			fmt.Sprintf(
				"%s you can change the search query by pressing %s and submitting it with %s",
				lipgloss.NewStyle().Bold(true).Render(" Tip:"),
				m.Ctx.Styles.Section.KeyStyle.Render("/"),
				m.Ctx.Styles.Section.KeyStyle.Render("Enter"),
			),
		)
	} else {
		return m.Table.View()
	}
}

func (m *BaseModel) View() string {
	search := m.SearchBar.View(m.Ctx)

	mainContent := m.GetMainContent()

	// If repo picker is shown, overlay it on the main content
	if m.IsRepoPickerShown {
		pickerView := m.RepoPicker.View()
		// Center the picker over the content
		d := m.GetDimensions()
		mainContent = lipgloss.Place(
			d.Width,
			d.Height,
			lipgloss.Center,
			lipgloss.Center,
			pickerView,
		)
	}

	return m.Ctx.Styles.Section.ContainerStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			search,
			mainContent,
		),
	)
}

func (m *BaseModel) ResetRows() {
	m.Table.Rows = nil
	m.ResetPageInfo()
	m.Table.ResetCurrItem()
}

func (m *BaseModel) LastUpdated() time.Time {
	return m.Table.LastUpdated()
}

func (m *BaseModel) CreatedAt() time.Time {
	return m.Table.CreatedAt()
}

func (m *BaseModel) UpdateTotalItemsCount(count int) {
	m.Table.UpdateTotalItemsCount(count)
}

func (m *BaseModel) GetPromptConfirmation() string {
	if m.IsPromptConfirmationShown {
		var prompt string
		switch {
		case m.PromptConfirmationAction == "close" && m.Ctx.View == config.PRsView:
			prompt = "Are you sure you want to close this PR? (Y/n) "

		case m.PromptConfirmationAction == "reopen" && m.Ctx.View == config.PRsView:
			prompt = "Are you sure you want to reopen this PR? (Y/n) "

		case m.PromptConfirmationAction == "ready" && m.Ctx.View == config.PRsView:
			prompt = "Are you sure you want to mark this PR as ready? (Y/n) "

		case m.PromptConfirmationAction == "merge" && m.Ctx.View == config.PRsView:
			prompt = "Are you sure you want to merge this PR? (Y/n) "

		case m.PromptConfirmationAction == "update" && m.Ctx.View == config.PRsView:
			prompt = "Are you sure you want to update this PR? (Y/n) "

		case m.PromptConfirmationAction == "close" && m.Ctx.View == config.IssuesView:
			prompt = "Are you sure you want to close this issue? (Y/n) "

		case m.PromptConfirmationAction == "reopen" && m.Ctx.View == config.IssuesView:
			prompt = "Are you sure you want to reopen this issue? (Y/n) "
		case m.PromptConfirmationAction == "delete" && m.Ctx.View == config.RepoView:
			prompt = "Are you sure you want to delete this branch? (Y/n) "
		case m.PromptConfirmationAction == "new" && m.Ctx.View == config.RepoView:
			prompt = "Enter branch name: "
		case m.PromptConfirmationAction == "create_pr" && m.Ctx.View == config.RepoView:
			prompt = "Enter PR title: "
		}

		m.PromptConfirmationBox.SetPrompt(prompt)

		return m.Ctx.Styles.ListViewPort.PagerStyle.Render(m.PromptConfirmationBox.View())
	}

	return ""
}
