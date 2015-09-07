IF (GOPATH)
  SET (_gopath "${GOPATH}")
  STRING (REPLACE ";" ":" _gopath "${_gopath}")
  SET (ENV{GOPATH} "${_gopath}")
ENDIF (GOPATH)

IF (COVER)
  GET_FILENAME_COMPONENT (_pkgname "${PACKAGE}" NAME)
  SET (_cover "-coverprofile=${_pkgname}.coverage.out")
ENDIF(COVER)

EXECUTE_PROCESS (RESULT_VARIABLE _failure
  COMMAND "${GO_EXECUTABLE}" test "${PACKAGE}" ${_cover})
IF (_failure)
  MESSAGE (FATAL_ERROR "Test ${PACKAGE} failed")
ENDIF (_failure)

IF (COVER)
  EXECUTE_PROCESS (RESULT_VARIABLE _failure
    COMMAND "${GO_EXECUTABLE}" tool cover "-html=${_pkgname}.coverage.out" -o "${_pkgname}.coverage.html")
  FILE(REMOVE ${_pkgname}.coverage.out)
ENDIF(COVER)
