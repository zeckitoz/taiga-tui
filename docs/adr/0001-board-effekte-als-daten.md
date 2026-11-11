# Board-State gibt Effekte als Daten zurück

Status Change und Take Over brauchen optimistische Änderungen, eine Sperre pro Item, eine Rücknahme bei Fehlern, ein Neuladen bei Konflikten und das Nachladen der Projektdaten bei unbekannten IDs.
Diese Logik liegt vollständig in `internal/board`, einer reinen Zustandsmaschine ohne Import von Bubble Tea oder HTTP.
Eine Aktion wie `ChangeStatus` gibt den neuen Zustand und einen Effekt als Daten zurück (z. B. `SaveStatus`, `ReloadBoard`, `ReloadProject`) oder eine Ablehnung.
Die Ergebnisse der Requests (`Saved`, `Conflict`, `NotFound`, `Failed`) gehen wieder als Eingabe in `board`.
`internal/ui` übersetzt Effekte nur in `tea.Cmd`s, die den API-Client aufrufen.
So ist die heikelste Logik ohne Terminal und ohne HTTP-Fake testbar und liegt an einer Stelle statt verteilt auf Taskboard und Kanban Board.

## Considered Options

- **UI-Models rufen den API-Client direkt in `Update` auf und machen den Rollback selbst.**
  Das ist weniger Code, aber die Regeln für Sperre, Rücknahme und Konflikt wären auf zwei Boards verteilt und nur über Bubble-Tea-Tests prüfbar.
  Abgelehnt.

## Consequences

- Ergebnisse tragen eine Generation, und `board` verwirft Ergebnisse veralteter Ladevorgänge.
  PATCH-Requests werden nicht abgebrochen, weil die Änderung auf dem Server trotzdem gilt.
- Wer neue Aktionen ergänzt, erweitert `board` um Effekt- und Ergebnistypen, statt Requests im UI-Code auszulösen.
