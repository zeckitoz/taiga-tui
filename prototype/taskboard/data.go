// PROTOTYP - Wegwerfcode für die Map-Frage "Wie sieht das Taskboard im Terminal aus?".
// Beispieldaten im Format der Taiga-API (Sprint, User Stories, Tasks, Status, Team Members).
package main

type Status struct {
	Name  string
	Color string
}

type Member struct {
	ID       int
	Name     string
	Initials string
}

type Story struct {
	Ref     int
	Subject string
}

type Task struct {
	Ref      int
	Subject  string
	Status   string // Name eines Status; im Modus "5 Status" auf baseStatus gemappt
	Assignee int    // Member.ID, 0 = kein Assignee
	Story    int    // Story.Ref, 0 = ohne User Story
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

// Taiga-6-Standardstatus für Tasks.
var baseStatuses = []Status{
	{"New", "#70728F"},
	{"In progress", "#E44057"},
	{"Ready for test", "#E47C40"},
	{"Closed", "#A8E440"},
	{"Needs Info", "#5178D3"},
}

// Projekt mit angepasstem Workflow: 8 Status, um Überlauf zu testen.
var manyStatuses = []Status{
	{"New", "#70728F"},
	{"Ready", "#8E9AAF"},
	{"In progress", "#E44057"},
	{"In review", "#9B59B6"},
	{"Ready for test", "#E47C40"},
	{"Blocked", "#C0392B"},
	{"Closed", "#A8E440"},
	{"Needs Info", "#5178D3"},
}

var collapseToBase = map[string]string{
	"Ready":     "New",
	"In review": "Ready for test",
	"Blocked":   "Needs Info",
}

const (
	projectName = "SE-Projekt WiSe 26/27 · Team 3"
	sprintName  = "Sprint 4"
	sprintDates = "03.11. - 16.11.2026"
)

var stories = []Story{
	{14, "Als Studierende möchte ich mich mit meiner Uni-Mail registrieren"},
	{17, "Freie Räume für einen Zeitraum anzeigen"},
	{21, "Gebuchte Termine als iCal exportieren"},
	{23, "Benachrichtigung bei Raumänderung"},
	{25, "Admin: Räume anlegen und pflegen"},
}

var tasks = []Task{
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
	{71, "CI-Pipeline mit Lint und Tests einrichten", "In progress", 4, 0, "GitHub Actions, Matrix für Node 22 und 24."},
	{72, "README und Setup-Anleitung aktualisieren", "New", 1, 0, "Docker-Compose-Setup beschreiben."},
	{73, "Retro-Notizen von Sprint 3 ins Wiki", "Closed", 5, 0, ""},
}

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
