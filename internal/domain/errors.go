package domain

import "errors"

// ErrDependencyUnavailable señala que el almacén de datos no está accesible
// (cliente desconectado, timeout de red, transporte roto). La capa de API lo
// traduce a 503 DEPENDENCY_UNAVAILABLE.
var ErrDependencyUnavailable = errors.New("el almacén de documentos no está disponible")
