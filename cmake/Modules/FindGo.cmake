# This module provides facilities for building Go code using either golang
# (preferred) or gccgo.
#
# This module defines
#  GO_FOUND, if a compiler was found

SET(GO_MINIMUM_VERSION 1.4)

# Prevent double-definition if two projects use this script
IF (NOT FindGo_INCLUDED)

  INCLUDE (ParseArguments)

  # Have to remember cwd when this find is INCLUDE()d
  SET (MODULES_DIR "${CMAKE_CURRENT_LIST_DIR}")

  # Figure out which Go compiler to use
  FIND_PROGRAM (GO_EXECUTABLE NAMES go DOC "Go executable")
  IF (GO_EXECUTABLE)
    EXECUTE_PROCESS (COMMAND ${GO_EXECUTABLE} version
                     OUTPUT_VARIABLE GO_VERSION_STRING)
    STRING (REGEX REPLACE "^go version go([0-9.]+).*$" "\\1" GO_VERSION ${GO_VERSION_STRING})
    # I've seen cases where the version contains a trailing newline
    STRING(STRIP "${GO_VERSION}" GO_VERSION)
    MESSAGE (STATUS "Found Go compiler: ${GO_EXECUTABLE} (${GO_VERSION})")

    IF(GO_VERSION VERSION_LESS GO_MINIMUM_VERSION)
      STRING(REGEX MATCH "^go version devel .*" go_dev_version "${GO_VERSION}")
      IF (go_dev_version)
          MESSAGE(STATUS "WARNING: You are using a development version of go")
          MESSAGE(STATUS "         Go version of ${GO_MINIMUM_VERSION} or higher required")
          MESSAGE(STATUS "         You may experience problems caused by this")
      ELSE(go_dev_version)
          MESSAGE(FATAL_ERROR "Go version of ${GO_MINIMUM_VERSION} or higher required (found version ${GO_VERSION})")
      ENDIF(go_dev_version)
    ENDIF(GO_VERSION VERSION_LESS GO_MINIMUM_VERSION)

    SET (GO_COMMAND_LINE "${GO_EXECUTABLE}" build -x)
    SET (GO_FOUND 1 CACHE BOOL "Whether Go compiler was found")

    IF (DEFINED ENV{GOBIN})
      MESSAGE(WARNING "The environment variable GOBIN is set and MAY cause your build to fail")
    ENDIF (DEFINED ENV{GOBIN})
  ELSE (GO_EXECUTABLE)
    FIND_PROGRAM (GCCGO_EXECUTABLE NAMES gccgo DOC "gccgo executable")
    IF (GCCGO_EXECUTABLE)
      MESSAGE (STATUS "Found gccgo compiler: ${GCCGO_EXECUTABLE}")
      SET (GO_COMMAND_LINE "${GCCGO_EXECUTABLE}" -Os -g)
      SET (GO_FOUND 1 CACHE BOOL "Whether Go compiler was found")
    ELSE (GCCGO_EXECUTABLE)
      SET (GO_FOUND 0)
    ENDIF (GCCGO_EXECUTABLE)
  ENDIF (GO_EXECUTABLE)


  # Call this macro in the beginning with PKGNAME is your gitlab url
  # and list Gitlab dependencies in DEPENDS
  MACRO(GoProjectConfigure)
    IF (NOT GO_EXECUTABLE)
      MESSAGE (FATAL_ERROR "Go compiler was not found!")
    ENDIF (NOT GO_EXECUTABLE)

    PARSE_ARGUMENTS (Go "DEPENDS" "PKGNAME" "NOCONSOLE" ${ARGN})

    IF (NOT Go_PKGNAME)
      MESSAGE(FATAL_ERROR "PKGNAME is required!")
    ENDIF (NOT Go_PKGNAME)

    # prepare GOPATHs
    IF (NOT USE_LOCAL_GOPATH)
      # sources has go-gotten, dependencies downloaded by go-get
      MESSAGE(STATUS "Using system GOPATH")
    ELSE (NOT USE_LOCAL_GOPATH)
      # sources signle git-clones, no dependencies
      # download it into local build-gopath
      SET(_gopath ${CMAKE_BINARY_DIR}/gopath)
      SET(ENV{GOPATH} ${_gopath})

      GET_FILENAME_COMPONENT(_source_dir ${Go_PKGNAME} PATH)
      IF(NOT EXISTS ${_gopath}/src/${Go_PKGNAME})
        FILE(MAKE_DIRECTORY ${_gopath}/src/${_source_dir})
        EXECUTE_PROCESS(RESULT_VARIABLE _lnFailure
          COMMAND ln -fs ${CMAKE_CURRENT_SOURCE_DIR} ${_gopath}/src/${Go_PKGNAME})
      ENDIF(NOT EXISTS ${_gopath}/src/${Go_PKGNAME})

      MESSAGE(STATUS "Using local GOPATH at ${_gopath}")
      MESSAGE(STATUS "go getting...")
      # go-get by hand private gitlab repo's
      FOREACH(_labRepo ${Go_DEPENDS})
        EXECUTE_PROCESS(RESULT_VARIABLE _getFailure
          COMMAND go get -d ${_labRepo}/...
          WORKING_DIRECTORY ${_gopath}/src/)
      ENDFOREACH(_labRepo ${Go_DEPENDS})

      # go-get foreign dependencies
      EXECUTE_PROCESS(RESULT_VARIABLE _getFailure
        COMMAND go get -d ./${Go_PKGNAME}/...
        WORKING_DIRECTORY ${_gopath}/src/)
    ENDIF (NOT USE_LOCAL_GOPATH)

    # ADD_CUSTOM_TARGET (check)
      # COMMAND "${CMAKE_COMMAND}"
      # -D "GOTESTS=${GOTESTS}"
      # -P "${MODULES_DIR}/go-test.cmake"
      # COMMENT "Running go test"
      # VERBATIM)

    SET(GO_CONFIGURED 1)
  ENDMACRO(GoProjectConfigure)

  # Adds a target named TARGET which (always) calls "go install
  # PACKAGE".  This delegates incremental-build responsibilities to
  # the go compiler, which is generally what you want.
  #
  # Note that this macro requires using the Golang compiler, not
  # gccgo. A CMake error will be raised if this macro is used when
  # only gccgo is detected.
  #
  # Required arguments:
  #
  # TARGET - name of CMake target to create
  #
  # PACKAGE - A single Go package to build. When this is specified,
  # the package and all dependencies on GOPATH will be built, using
  # the Go compiler's normal dependency-handling system.
  #
  # GOPATH - Every entry on this list will be placed onto the GOPATH
  # environment variable before invoking the compiler.
  #
  # Optional arguments:
  #
  # GCFLAGS - flags that will be passed (via -gcflags) to all compile
  # steps; should be a single string value, with spaces if necessary
  #
  # GOTAGS - tags that will be passed (viga -tags) to all compile
  # steps; should be a single string value, with spaces as necessary
  #
  # LDFLAGS - flags that will be passed (via -ldflags) to all compile
  # steps; should be a single string value, with spaces if necessary
  #
  # NOCONSOLE - for targets that should not launch a console at runtime
  # (on Windows - silently ignored on other platforms)
  #
  # DEPENDS - list of other CMake targets on which TARGET will depend
  #
  # INSTALL_PATH - if specified, a CMake INSTALL() directive will be
  # created to install the output into the named path
  #
  # OUTPUT - name of the installed executable (only applicable if
  # INSTALL_PATH is specified). Default value is the basename of
  # PACKAGE, per the go compiler. On Windows, ".exe" will be
  # appended.
  #
  # CGO_INCLUDE_DIRS - path(s) to directories to search for C include files
  #
  # CGO_LIBRARY_DIRS - path(s) to libraries to search for C link libraries
  #
  MACRO (GoInstall)
    IF (NOT GO_EXECUTABLE)
      MESSAGE (FATAL_ERROR "Go compiler was not found!")
    ENDIF (NOT GO_EXECUTABLE)

    IF (NOT GO_CONFIGURED)
      MESSAGE (FATAL_ERROR "Go cmake project is not configured, use macro GoProjectConfigure")
    ENDIF (NOT GO_CONFIGURED)

    PARSE_ARGUMENTS (Go "DEPENDS;GOPATH;CGO_INCLUDE_DIRS;CGO_LIBRARY_DIRS"
      "TARGET;PACKAGE;OUTPUT;INSTALL_PATH;GCFLAGS;GOTAGS;LDFLAGS" "NOCONSOLE" ${ARGN})

    IF (NOT Go_TARGET)
      MESSAGE (FATAL_ERROR "TARGET is required!")
    ENDIF (NOT Go_TARGET)
    IF (NOT Go_PACKAGE)
      MESSAGE (FATAL_ERROR "PACKAGE is required!")
    ENDIF (NOT Go_PACKAGE)

    # Extract the binary name from the package.
    GET_FILENAME_COMPONENT (_pkgexe "${Go_PACKAGE}" NAME)

    # prepare GOPATHs
    IF (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH $ENV{GOPATH})
    ELSE (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH ${CMAKE_BINARY_DIR}/gopath)
    ENDIF (NOT USE_LOCAL_GOPATH)

    # Hunt for the requested package on GOPATH (used for installing)
    SET (_found)
    FOREACH (_dir ${Go_GOPATH})
      FILE (TO_NATIVE_PATH "${_dir}/src/${Go_PACKAGE}" _pkgdir)
      IF (IS_DIRECTORY "${_pkgdir}")
        SET (_found 1)
        SET (_workspace "${_dir}")
        BREAK ()
      ENDIF (IS_DIRECTORY "${_pkgdir}")
    ENDFOREACH (_dir)
    IF (NOT _found)
      MESSAGE (FATAL_ERROR "Package ${Go_PACKAGE} not found in any workspace on GOPATH!")
    ENDIF (NOT _found)

    # Go install target
    ADD_CUSTOM_TARGET ("${Go_TARGET}" ALL
      COMMAND "${CMAKE_COMMAND}"
      -D "GO_EXECUTABLE=${GO_EXECUTABLE}"
      -D CMAKE_C_COMPILER=${CMAKE_C_COMPILER}
      -D "GOPATH=${Go_GOPATH}"
      -D "WORKSPACE=${_workspace}"
      -D "CGO_LDFLAGS=${CMAKE_CGO_LDFLAGS}"
      -D "GCFLAGS=${Go_GCFLAGS}"
      -D "GOTAGS=${Go_GOTAGS}"
      -D "LDFLAGS=${_ldflags}"
      -D "PKGEXE=${_pkgexe}"
      -D "PACKAGE=${Go_PACKAGE}"
      -D "OUTPUT=${Go_OUTPUT}"
      -D "CGO_INCLUDE_DIRS=${Go_CGO_INCLUDE_DIRS}"
      -D "CGO_LIBRARY_DIRS=${Go_CGO_LIBRARY_DIRS}"
      -P "${MODULES_DIR}/go-install.cmake"
      COMMENT "Building Go target ${Go_PACKAGE}"
      VERBATIM)
    IF (Go_DEPENDS)
      ADD_DEPENDENCIES (${Go_TARGET} ${Go_DEPENDS})
    ENDIF (Go_DEPENDS)

    # We expect multiple go targets to be operating over the same
    # GOPATH.  It seems like the go compiler doesn't like be invoked
    # in parallel in this case, as would happen if we parallelize the
    # Couchbase build (eg., 'make -j8'). Since the go compiler itself
    # does parallel building, we want to serialize all go targets. So,
    # we make them all depend on any earlier Go targets.
    GET_PROPERTY (_go_targets GLOBAL PROPERTY CB_GO_TARGETS)
    IF (_go_targets)
      ADD_DEPENDENCIES(${Go_TARGET} ${_go_targets})
    ENDIF (_go_targets)
    SET_PROPERTY (GLOBAL APPEND PROPERTY CB_GO_TARGETS ${Go_TARGET})

    # Tweaks for installing and output renaming. go-install.cmake will
    # arrange for the workspace's bin directory to contain a file with
    # the right name (either OUTPUT, or the Go package name if OUTPUT
    # is not specified). We need to know what that name is so we can
    # INSTALL() it.
    IF (Go_OUTPUT)
      SET (_finalexe "${Go_OUTPUT}")
    ELSE (Go_OUTPUT)
      SET (_finalexe "${_pkgexe}")
    ENDIF (Go_OUTPUT)
    IF (Go_INSTALL_PATH)
      INSTALL (PROGRAMS "${_workspace}/bin/${_finalexe}"
        DESTINATION "${Go_INSTALL_PATH}")
    ENDIF (Go_INSTALL_PATH)

  ENDMACRO (GoInstall)


  # Adds a target named TARGET which (always) calls "go tool yacc
  # PATH".
  #
  # Note that this macro requires using the Golang compiler, not
  # gccgo. A CMake error will be raised if this macro is used when
  # only gccgo is detected.
  #
  # Required arguments:
  #
  # TARGET - name of CMake target to create
  #
  # YFILE - Absolute path to .y file.
  #
  # Optional arguments:
  #
  # DEPENDS - list of other CMake targets on which TARGET will depend
  #
  #
  MACRO (GoYacc)
    IF (NOT GO_EXECUTABLE)
      MESSAGE (FATAL_ERROR "Go compiler was not found!")
    ENDIF (NOT GO_EXECUTABLE)

    PARSE_ARGUMENTS (Go "DEPENDS" "TARGET;YFILE" "" ${ARGN})

    IF (NOT Go_TARGET)
      MESSAGE (FATAL_ERROR "TARGET is required!")
    ENDIF (NOT Go_TARGET)
    IF (NOT Go_YFILE)
      MESSAGE (FATAL_ERROR "YFILE is required!")
    ENDIF (NOT Go_YFILE)

    GET_FILENAME_COMPONENT (_ypath "${Go_YFILE}" PATH)
    GET_FILENAME_COMPONENT (_yname "${Go_YFILE}" NAME)

    SET(Go_OUTPUT "${_ypath}/y.go")

    ADD_CUSTOM_COMMAND(OUTPUT "${Go_OUTPUT}"
                       COMMAND "${GO_EXECUTABLE}" tool yacc "${_yname}"
                       COMMENT "Executing: ${GO_EXECUTABLE} tool yacc ${_yname}"
                       DEPENDS ${Go_YFILE}
                       WORKING_DIRECTORY "${_ypath}"
                       VERBATIM)

    ADD_CUSTOM_TARGET ("${Go_TARGET}"
                       DEPENDS "${Go_OUTPUT}")

    IF (Go_DEPENDS)
      ADD_DEPENDENCIES (${Go_TARGET} ${Go_DEPENDS})
    ENDIF (Go_DEPENDS)

  ENDMACRO (GoYacc)

  SET (FindGo_INCLUDED 1)

ENDIF (NOT FindGo_INCLUDED)


  # Adds a target named TARGET which (always) calls "go install
  # PACKAGE".  This delegates incremental-build responsibilities to
  # the go compiler, which is generally what you want.
  #
  # Note that this macro requires using the Golang compiler, not
  # gccgo. A CMake error will be raised if this macro is used when
  # only gccgo is detected.
  #
  # Required arguments:
  #
  # TARGET - name of CMake target to create
  #
  # PACKAGE - A single Go package to build. When this is specified,
  # the package and all dependencies on GOPATH will be built, using
  # the Go compiler's normal dependency-handling system.
  #
  # GOPATH - Every entry on this list will be placed onto the GOPATH
  # environment variable before invoking the compiler.
  #
  # Optional arguments:
  #
  # GCFLAGS - flags that will be passed (via -gcflags) to all compile
  # steps; should be a single string value, with spaces if necessary
  #
  # GOTAGS - tags that will be passed (viga -tags) to all compile
  # steps; should be a single string value, with spaces as necessary
  #
  # LDFLAGS - flags that will be passed (via -ldflags) to all compile
  # steps; should be a single string value, with spaces if necessary
  #
  # NOCONSOLE - for targets that should not launch a console at runtime
  # (on Windows - silently ignored on other platforms)
  #
  # DEPENDS - list of other CMake targets on which TARGET will depend
  #
  # INSTALL_PATH - if specified, a CMake INSTALL() directive will be
  # created to install the output into the named path
  #
  # OUTPUT - name of the installed executable (only applicable if
  # INSTALL_PATH is specified). Default value is the basename of
  # PACKAGE, per the go compiler. On Windows, ".exe" will be
  # appended.
  #
  # CGO_INCLUDE_DIRS - path(s) to directories to search for C include files
  #
  # CGO_LIBRARY_DIRS - path(s) to libraries to search for C link libraries
  #
  MACRO (GoBuild)
    IF (NOT GO_EXECUTABLE)
      MESSAGE (FATAL_ERROR "Go compiler was not found!")
    ENDIF (NOT GO_EXECUTABLE)

    IF (NOT GO_CONFIGURED)
      MESSAGE (FATAL_ERROR "Go cmake project is not configured, use macro GoProjectConfigure")
    ENDIF (NOT GO_CONFIGURED)

    PARSE_ARGUMENTS (Go "DEPENDS;GOPATH;CGO_INCLUDE_DIRS;CGO_LIBRARY_DIRS"
      "TARGET;PACKAGE;OUTPUT;INSTALL_PATH;GCFLAGS;GOTAGS;LDFLAGS" "NOCONSOLE" ${ARGN})

    IF (NOT Go_TARGET)
      MESSAGE (FATAL_ERROR "TARGET is required!")
    ENDIF (NOT Go_TARGET)
    IF (NOT Go_PACKAGE)
      MESSAGE (FATAL_ERROR "PACKAGE is required!")
    ENDIF (NOT Go_PACKAGE)

    # Extract the binary name from the package.
    GET_FILENAME_COMPONENT (_pkgexe "${Go_PACKAGE}" NAME)

    # prepare GOPATHs
    IF (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH $ENV{GOPATH})
    ELSE (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH ${CMAKE_BINARY_DIR}/gopath)
    ENDIF (NOT USE_LOCAL_GOPATH)

    # Hunt for the requested package on GOPATH (used for installing)
    SET (_found)
    FOREACH (_dir ${Go_GOPATH})
      FILE (TO_NATIVE_PATH "${_dir}/src/${Go_PACKAGE}" _pkgdir)
      IF (IS_DIRECTORY "${_pkgdir}")
        SET (_found 1)
        SET (_workspace "${_dir}")
        BREAK ()
      ENDIF (IS_DIRECTORY "${_pkgdir}")
    ENDFOREACH (_dir)
    IF (NOT _found)
      MESSAGE (FATAL_ERROR "Package ${Go_PACKAGE} not found in any workspace on GOPATH!")
    ENDIF (NOT _found)

    IF (NOT Go_OUTPUT)
      SET(Go_OUTPUT ${PROJECT_NAME})
    ENDIF (NOT Go_OUTPUT)

    # Go install target
    ADD_CUSTOM_TARGET ("${Go_TARGET}" ALL
      COMMAND "${CMAKE_COMMAND}"
      -D "GO_EXECUTABLE=${GO_EXECUTABLE}"
      -D CMAKE_C_COMPILER=${CMAKE_C_COMPILER}
      -D "GOPATH=${Go_GOPATH}"
      -D "WORKSPACE=${_workspace}"
      -D "CGO_LDFLAGS=${CMAKE_CGO_LDFLAGS}"
      -D "GCFLAGS=${Go_GCFLAGS}"
      -D "GOTAGS=${Go_GOTAGS}"
      -D "LDFLAGS=${_ldflags}"
      -D "PKGEXE=${_pkgexe}"
      -D "PACKAGE=${Go_PACKAGE}"
      -D "OUTPUT=${Go_OUTPUT}"
      -D "CGO_INCLUDE_DIRS=${Go_CGO_INCLUDE_DIRS}"
      -D "CGO_LIBRARY_DIRS=${Go_CGO_LIBRARY_DIRS}"
      -P "${MODULES_DIR}/go-build.cmake"
      COMMENT "Building Go target ${Go_PACKAGE}"
      VERBATIM)
    IF (Go_DEPENDS)
      ADD_DEPENDENCIES (${Go_TARGET} ${Go_DEPENDS})
    ENDIF (Go_DEPENDS)

    # We expect multiple go targets to be operating over the same
    # GOPATH.  It seems like the go compiler doesn't like be invoked
    # in parallel in this case, as would happen if we parallelize the
    # Couchbase build (eg., 'make -j8'). Since the go compiler itself
    # does parallel building, we want to serialize all go targets. So,
    # we make them all depend on any earlier Go targets.
    GET_PROPERTY (_go_targets GLOBAL PROPERTY CB_GO_TARGETS)
    IF (_go_targets)
      ADD_DEPENDENCIES(${Go_TARGET} ${_go_targets})
    ENDIF (_go_targets)
    SET_PROPERTY (GLOBAL APPEND PROPERTY CB_GO_TARGETS ${Go_TARGET})

    # Tweaks for installing and output renaming. go-install.cmake will
    # arrange for the workspace's bin directory to contain a file with
    # the right name (either OUTPUT, or the Go package name if OUTPUT
    # is not specified). We need to know what that name is so we can
    # INSTALL() it.
    IF (Go_OUTPUT)
      SET (_finalexe "${Go_OUTPUT}")
    ELSE (Go_OUTPUT)
      SET (_finalexe "${_pkgexe}")
    ENDIF (Go_OUTPUT)
    IF (Go_INSTALL_PATH)
      INSTALL (PROGRAMS ${CMAKE_CURRENT_BINARY_DIR}/${Go_OUTPUT}
        DESTINATION "${Go_INSTALL_PATH}")
    ENDIF (Go_INSTALL_PATH)

  ENDMACRO (GoBuild)


  MACRO(GoTest)
    IF (NOT GO_EXECUTABLE)
      MESSAGE (FATAL_ERROR "Go compiler was not found!")
    ENDIF (NOT GO_EXECUTABLE)

    IF (NOT GO_CONFIGURED)
      MESSAGE (FATAL_ERROR "Go cmake project is not configured, use macro GoProjectConfigure")
    ENDIF (NOT GO_CONFIGURED)

    PARSE_ARGUMENTS (Go "PACKAGES" "" "NOCONSOLE" ${ARGN})


    # prepare GOPATHs
    IF (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH $ENV{GOPATH})
    ELSE (NOT USE_LOCAL_GOPATH)
      SET(Go_GOPATH ${CMAKE_BINARY_DIR}/gopath)
    ENDIF (NOT USE_LOCAL_GOPATH)

    FOREACH(_pkg ${Go_PACKAGES})
      ADD_TEST(
        NAME ${_pkg}_test
        COMMAND ${CMAKE_COMMAND}
          -D "PACKAGE=${_pkg}"
          -D "GOPATH=${Go_GOPATH}"
          -D "GO_EXECUTABLE=${GO_EXECUTABLE}"
          -D "COVER=${GO_COVER}"
          -P ${MODULES_DIR}/go-test.cmake
      )

      # LIST(APPEND GOTESTS ${_pkg})
      # ADD_CUSTOM_COMMAND(TARGET check
        # COMMAND echo ${_pkg})
      # ADD_DEPENDENCIES(check generate_foo)
    ENDFOREACH(_pkg ${Go_PACKAGES})

    # set_target_properties(check PROPERTIES GOTESTS ${GOTESTS})

    # FOREACH(_pkg ${GOTESTS})
    #   ADD_CUSTOM_TARGET(check
    #     COMMAND echo ${_pkg}
    #   )

    #   # ADD_TEST(NAME ${_pkg}_test COMMAND go test ${_pkg})
    #   # EXECUTE_PROCESS(RESULT_VARIABLE _abc COMMAND go test ${_pkg})
    # ENDFOREACH(_pkg ${GOTESTS})
  ENDMACRO(GoTest)
