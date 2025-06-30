// Package clocks implementa un reloj vectorial ligero, thread-safe y
// fácil de serializar a una slice de enteros o a un string “k=v,k=v,…”.
package clocks

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Vector almacena el reloj en un mapa id->contador y un mutex para concurrencia.
type Vector struct {
	mu    sync.Mutex
	clock map[string]int64
}

// New crea un reloj con todos los ids inicializados en 0.
func New(ids ...string) *Vector {
	c := make(map[string]int64, len(ids))
	for _, id := range ids {
		c[id] = 0
	}
	return &Vector{clock: c}
}

// Copy devuelve una copia profunda (independiente) del reloj.
func (v *Vector) Copy() *Vector {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make(map[string]int64, len(v.clock))
	for k, val := range v.clock {
		out[k] = val
	}
	return &Vector{clock: out}
}

// Tick incrementa el contador del id local y devuelve el nuevo valor.
func (v *Vector) Tick(id string) int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.clock[id]++
	return v.clock[id]
}

// Merge fusiona otro reloj en el actual aplicando max por componente.
func (v *Vector) Merge(other *Vector) {
	v.mu.Lock()
	defer v.mu.Unlock()
	other.mu.Lock()
	defer other.mu.Unlock()

	for id, val := range other.clock {
		if cur, ok := v.clock[id]; !ok || val > cur {
			v.clock[id] = val
		}
	}
}

// HappensBefore devuelve true si v < other en el orden parcial de relojes
// (v ≤ other y al menos un componente estrictamente menor).
func (v *Vector) HappensBefore(other *Vector) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	other.mu.Lock()
	defer other.mu.Unlock()

	less := false
	for id, val := range v.clock {
		o := other.clock[id]
		if val > o {
			return false
		}
		if val < o {
			less = true
		}
	}
	// Si other tiene ids que v no posee y alguno es >0, v también es menor.
	for id, oval := range other.clock {
		if _, ok := v.clock[id]; !ok && oval > 0 {
			less = true
		}
	}
	return less
}

// String serializa a "id1=3,id2=1".
func (v *Vector) String() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	parts := make([]string, 0, len(v.clock))
	for id, val := range v.clock {
		parts = append(parts, fmt.Sprintf("%s=%d", id, val))
	}
	return strings.Join(parts, ",")
}

// FromString parsea el formato "id1=3,id2=1" y sobreescribe el reloj.
func (v *Vector) FromString(s string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s == "" {
		return nil
	}
	for _, kv := range strings.Split(s, ",") {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("bad vector clock component: %q", kv)
		}
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return err
		}
		v.clock[parts[0]] = val
	}
	return nil
}

// ToSlice devuelve ids y valores paralelos (útil para serializar en proto).
func (v *Vector) ToSlice() (ids []string, values []int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id, val := range v.clock {
		ids = append(ids, id)
		values = append(values, val)
	}
	return
}
