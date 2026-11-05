package i18n

import "encoding/json"

// Cat is words the app puts together: Msg sentences and plain strings (a value the data carries, a separator)
// said one after the other, as {"$": "cat", "v": [...]}. The server sends the pieces and the app does the joining,
// so no sentence is composed here. A Cat with nothing in it is null.
type Cat []any

func (c Cat) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(map[string]any{"$": "cat", "v": []any(c)})
}
