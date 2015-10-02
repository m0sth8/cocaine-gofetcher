CMAKE_MINIMUM_REQUIRED(VERSION 2.8.2)
PROJECT(gofetcher)

MACRO(LOCATE_PATH VARIABLE FILE_NAME)
    IF(${VARIABLE}_TMP AND ${VARIABLE}_PATH)
        RETURN()
    ENDIF()
     
    FIND_PATH(${VARIABLE}_TMP NAMES ${FILE_NAME})
     
    IF(${VARIABLE}_TMP)
        SET(${VARIABLE}_PATH ${${VARIABLE}_TMP}/${FILE_NAME})
        MESSAGE(STATUS "${FILE_NAME} at: ${${VARIABLE}_PATH}")
    ELSE()
        MESSAGE(FATAL_ERROR "${FILE_NAME} not found.")
    ENDIF()
ENDMACRO()

LOCATE_PATH(GO_MODULE_CMAKE share/sputnik/gopack/cmake)
SET(CMAKE_MODULE_PATH ${CMAKE_MODULE_PATH} ${GO_MODULE_CMAKE_PATH})
INCLUDE(FindGo)

GoProjectConfigure(
    PKGNAME github.com/m0sth8/cocaine-gofetcher
)

IF(NOT CMAKE_BUILD_TYPE MATCHES "ReleaseWithDebInfo")
	set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -W -Wall -Wextra -pthread -std=c++14 -Wl,--no-as-needed")
ELSE()
	set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS_RELWITHDEBINFO} -W -Wall -Wextra -pthread -std=c++14 -Wl,--no-as-needed")
ENDIF()

ENABLE_TESTING()
GoInstall(
    TARGET app 
    PACKAGE github.com/m0sth8/cocaine-gofetcher/cmd/gofetcher
)
#GoTest(
#    PACKAGES github.com/m0sth8/cocaine-gofetcher/test
#)

INSTALL(
    PROGRAMS ${CMAKE_BINARY_DIR}/gopath/bin/${PROJECT_NAME}
    DESTINATION bin COMPONENT runtime
)

CONFIGURE_FILE(${CMAKE_CURRENT_SOURCE_DIR}/app/gofetcher/cocaine.cmake.in ${CMAKE_CURRENT_BINARY_DIR}/cocaine.cmake)
IF(NOT CMAKE_BUILD_TYPE MATCHES "ReleaseWithDebInfo")
    INSTALL(SCRIPT ${CMAKE_CURRENT_BINARY_DIR}/cocaine.cmake)
ENDIF()
