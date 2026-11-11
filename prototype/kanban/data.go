// PROTOTYPE - throwaway code for the map question
// "Wie sehen das Kanban Board und der Wechsel zwischen den Boards aus?".
// Sample data shaped like the Taiga API: projects, sprints, User Stories, Tasks, Statuses, Team Members.
package main

type Status struct {
	Name  string
	Color string
	WIP   int // WIP limit (only User Story statuses), 0 = none; not enforced by Taiga
}

type Member struct {
	ID       int
	Name     string
	Initials string
}

type Project struct {
	Name    string
	Backlog bool // is_backlog_activated: Sprints and Taskboard exist
	Kanban  bool // is_kanban_activated
	Data    bool // prototype: only the first project has sample data
}

type Sprint struct {
	Name   string
	Dates  string
	Closed bool
}

// Story is a User Story. Sprint is an index into sprints, -1 = not in a Sprint.
type Story struct {
	Ref       int
	Subject   string
	Status    string // User Story Status
	Assignees []int
	Sprint    int
	Desc      string
}

type Task struct {
	Ref      int
	Subject  string
	Status   string // Task Status; mapped to baseStatuses unless "8 statuses" is on
	Assignee int    // Member.ID, 0 = no Assignee
	Story    int    // Story.Ref, 0 = no User Story
	Desc     string
}

const meID = 1

var members = []Member{
	{1, "Lena Vogt", "LV"},
	{2, "Jonas Becker", "JB"},
	{3, "Aylin Demir", "AD"},
	{4, "Tim Schröder", "TS"},
	{5, "Mira Hoffmann", "MH"},
}

var projects = []Project{
	{"SE-Projekt WiSe 26/27 · Team 3", true, true, true},
	{"Abschlussarbeit Doku", false, true, false},
	{"Praktikum Rechnernetze Gr. B", true, false, false},
}

const defaultProject = 0

var sprints = []Sprint{
	{"Sprint 3", "20.10. - 02.11.2026", true},
	{"Sprint 4", "03.11. - 16.11.2026", false},
	{"Sprint 5", "17.11. - 30.11.2026", false},
}

const currentSprint = 1 // date range contains today (11.11.2026)

// Taiga 6 default User Story statuses (Archived is not loaded: status__is_archived=false).
var usStatuses = []Status{
	{"New", "#999999", 0},
	{"Ready", "#FF8A84", 0},
	{"In progress", "#FF9900", 3},
	{"Ready for test", "#FCC000", 0},
	{"Done", "#669900", 0},
}

// Taiga 6 default Task statuses.
var baseStatuses = []Status{
	{"New", "#70728F", 0},
	{"In progress", "#E44057", 0},
	{"Ready for test", "#E47C40", 0},
	{"Closed", "#A8E440", 0},
	{"Needs Info", "#5178D3", 0},
}

// Project with a custom Task workflow: 8 statuses.
var manyStatuses = []Status{
	{"New", "#70728F", 0},
	{"Ready", "#8E9AAF", 0},
	{"In progress", "#E44057", 0},
	{"In review", "#9B59B6", 0},
	{"Ready for test", "#E47C40", 0},
	{"Blocked", "#C0392B", 0},
	{"Closed", "#A8E440", 0},
	{"Needs Info", "#5178D3", 0},
}

var collapseToBase = map[string]string{
	"Ready":     "New",
	"In review": "Ready for test",
	"Blocked":   "Needs Info",
}

// All User Stories of the project in kanban_order.
var stories = []Story{
	{5, "Login mit Uni-Account", "Done", []int{1, 2}, 0, "Login über das bestehende Taiga-Konto, Session 24 h."},
	{8, "Projektgerüst und Deployment auf den Staging-Server", "Done", []int{4, 5}, 0, "Docker Compose, Staging unter staging.team3.example."},
	{14, "Als Studierende möchte ich mich mit meiner Uni-Mail registrieren", "In progress", []int{1, 2, 3}, 1, "Registrierung nur mit Adressen der Hochschule. Bestätigung per Mail."},
	{17, "Freie Räume für einen Zeitraum anzeigen", "In progress", []int{3, 4}, 1, "Suche nach Gebäude, Kapazität und Zeitraum."},
	{83, "Raumbelegung als Wochenansicht", "In progress", []int{1}, -1, "Kalenderansicht Mo-Fr, 8-20 Uhr."},
	{85, "Raumfotos hochladen", "In progress", []int{5}, -1, "Max. 5 Fotos pro Raum, automatisch verkleinert."},
	{21, "Gebuchte Termine als iCal exportieren", "Ready for test", []int{5, 1}, 1, "Export aller eigenen Buchungen als .ics."},
	{23, "Benachrichtigung bei Raumänderung", "Ready", nil, 1, "Mail an alle Teilnehmenden, wenn sich der Raum ändert."},
	{29, "Buchung stornieren", "Ready", []int{2}, 2, "Bis 2 h vor Beginn ohne Rückfrage."},
	{25, "Admin: Räume anlegen und pflegen", "Done", []int{2}, 1, "CRUD für Räume, nur Rolle admin."},
	{27, "Raum als Favorit merken", "New", nil, 2, ""},
	{80, "Barrierefreiheit der Buchungsseite prüfen", "New", nil, -1, "WCAG 2.2 AA, Tastaturbedienung."},
	{81, "Dark Mode für die Web-App", "New", []int{3}, -1, ""},
	{82, "Mehrsprachigkeit DE/EN", "New", nil, -1, "Texte in Ressourcendateien auslagern."},
	{84, "Push-Benachrichtigung in der PWA", "New", nil, -1, ""},
	{86, "Lasttest vor der Abgabe", "New", []int{4}, -1, "k6, 200 parallele Nutzer."},
}

var tasks = []Task{
	{6, "Login-Formular", "Closed", 1, 5, ""},
	{7, "Session-Handling", "Closed", 2, 5, ""},
	{9, "Dockerfile und Compose", "Closed", 4, 8, ""},
	{10, "Staging-Server einrichten", "Closed", 5, 8, ""},
	{31, "Formular für Registrierung bauen", "Closed", 1, 14, "Felder: Name, Uni-Mail, Passwort. Validierung clientseitig."},
	{32, "E-Mail-Domain auf ostfalia.de prüfen", "In progress", 1, 14, "Nur Adressen mit @ostfalia.de zulassen. Subdomains wie stud.ostfalia.de ebenfalls."},
	{33, "Bestätigungsmail versenden", "In review", 2, 14, "SMTP über den Uni-Relay. Link ist 24 h gültig."},
	{34, "Tests für Registrierungs-Endpoint", "New", 0, 14, "Happy Path, doppelte Mail, falsche Domain."},
	{35, "Passwortregeln mit Datenschutz abstimmen", "Needs Info", 3, 14, "Rückfrage an den Betreuer offen: Mindestlänge 12 oder 10?"},
	{41, "Abfrage freier Räume im Backend", "Closed", 4, 17, "Overlapping-Query auf bookings, Index auf (room_id, start)."},
	{42, "Filter nach Gebäude und Kapazität", "In progress", 3, 17, "Dropdown für Gebäude, Slider für Kapazität."},
	{43, "Zeitraumauswahl mit Datepicker", "In progress", 1, 17, "Bibliothek klären, möglichst ohne extra Dependency."},
	{44, "Leere Ergebnisliste sinnvoll darstellen", "Ready", 0, 17, "Hinweis plus Vorschlag für den nächsten freien Slot."},
	{45, "Performance mit 400 Räumen messen", "Blocked", 4, 17, "Wartet auf Testdaten aus dem Raumverwaltungssystem."},
	{46, "API-Doku für /rooms/free", "New", 2, 17, "OpenAPI-Abschnitt ergänzen."},
	{51, "iCal-Format recherchieren (RFC 5545)", "Closed", 5, 21, "VEVENT mit UID, DTSTART, DTEND, LOCATION."},
	{52, "Export-Endpoint /bookings.ics", "Ready for test", 5, 21, "Content-Type text/calendar, Zeitzone Europe/Berlin."},
	{53, "Download-Button im Profil", "Ready for test", 1, 21, "Button neben 'Meine Buchungen'."},
	{54, "Import in Outlook und Thunderbird testen", "New", 0, 21, "Mindestens Outlook Web und Thunderbird 128."},
	{61, "Räume anlegen (Formular)", "Closed", 2, 25, "Name, Gebäude, Kapazität, Ausstattung."},
	{62, "Räume bearbeiten und löschen", "Closed", 2, 25, "Soft Delete, damit alte Buchungen erhalten bleiben."},
	{63, "Rechteprüfung für Admin-Routen", "Closed", 4, 25, "Rolle 'admin' aus der Session."},
	{28, "Storno-Endpoint", "New", 0, 29, ""},
	{30, "Storno-Button in 'Meine Buchungen'", "New", 0, 29, ""},
	{71, "CI-Pipeline mit Lint und Tests einrichten", "In progress", 4, 0, "GitHub Actions, Matrix für Node 22 und 24."},
	{72, "README und Setup-Anleitung aktualisieren", "New", 1, 0, "Docker-Compose-Setup beschreiben."},
	{73, "Retro-Notizen von Sprint 3 ins Wiki", "Closed", 5, 0, ""},
}

// orphanSprint: Tasks without a User Story belong to this Sprint in the sample data.
const orphanSprint = 1

func memberByID(id int) *Member {
	for i := range members {
		if members[i].ID == id {
			return &members[i]
		}
	}
	return nil
}

func storyByRef(ref int) *Story {
	for i := range stories {
		if stories[i].Ref == ref {
			return &stories[i]
		}
	}
	return nil
}

func (s Story) mine() bool {
	for _, a := range s.Assignees {
		if a == meID {
			return true
		}
	}
	return false
}

// taskProgress counts closed and total Tasks of a User Story.
func taskProgress(ref int) (done, total int) {
	for _, t := range tasks {
		if t.Story == ref {
			total++
			if t.Status == "Closed" {
				done++
			}
		}
	}
	return
}
