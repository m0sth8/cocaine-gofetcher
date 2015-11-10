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


GoPackSelf(
    PKGNAME github.com/m0sth8/cocaine-gofetcher/gogen
    OUTPUT ${CMAKE_SOURCE_DIR}/gogen
    METHODS
        methods
    SPECS
    MODULE 99
)
GoProjectConfigure(
    PKGNAME github.com/m0sth8/cocaine-gofetcher
    DEPENDS
        gitlab.srv.pv.km/common/schema
)

IF(NOT CMAKE_BUILD_TYPE MATCHES "ReleaseWithDebInfo")
	set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -W -Wall -Wextra -pthread -std=c++14 -Wl,--no-as-needed")
ELSE()
	set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS_RELWITHDEBINFO} -W -Wall -Wextra -pthread -std=c++14 -Wl,--no-as-needed")
ENDIF()

set(SPEC_INCLUDE_DIR ${CMAKE_CURRENT_BINARY_DIR}/gen/specs)
LOCATE_PATH(GET_VERSION_CMAKE share/sputnik/schema/cmake/get_version.cmake)
LOCATE_PATH(AUTOJSONCXX_TEMPLATE share/autojsoncxx/code_template)
LOCATE_PATH(STRUCT_TEMPLATE share/sputnik/schema/struct_template)
LOCATE_PATH(AUTOJSONCXX_GENERATOR autojsoncxx.py)
LOCATE_PATH(STUBS_GENERATOR_CMAKE share/sputnik/schema/cmake/generate_stubs.cmake)
LOCATE_PATH(TYPE_TRAITS_CODE_TEMPLATE share/sputnik/schema/type_traits_code_template)
LOCATE_PATH(SCHEMA_CODE_TEMPLATE share/sputnik/schema/schema_code_template)
LOCATE_PATH(SCHEMA_GENERATOR json_meta_generator.py)
INCLUDE(${STUBS_GENERATOR_CMAKE_PATH})
INCLUDE(${GET_VERSION_CMAKE_PATH})

GenerateExternalStubs(
                ${CMAKE_CURRENT_SOURCE_DIR}/methods
                ${SPEC_INCLUDE_DIR}
                ${STRUCT_TEMPLATE_PATH}
                ${AUTOJSONCXX_GENERATOR_PATH}
                ${SCHEMA_GENERATOR_PATH}
                ${SCHEMA_CODE_TEMPLATE_PATH}
                ${TYPE_TRAITS_CODE_TEMPLATE_PATH}
)

GET_VERSION()
SET(APP_VERSION_MAJOR ${MAJOR_VERSION})
SET(APP_VERSION_MINOR ${MINOR_VERSION})
SET(APP_VERSION_PATCH ${PATCH_VERSION})
SET(APP_VERSION_BUILD ${BUILD_VERSION})

CONFIGURE_FILE(
    "${PROJECT_SOURCE_DIR}/app_version.hpp.in"
    "${PROJECT_SOURCE_DIR}/gogen/${PROJECT_NAME}/version/version.go")

install(
  DIRECTORY ${SPEC_INCLUDE_DIR}/
  COMPONENT development
  DESTINATION include/sputnik/gofetcher/
  FILES_MATCHING
  PATTERN "*.hpp")

ENABLE_TESTING()
GoBuild(
    TARGET app 
    PACKAGE github.com/m0sth8/cocaine-gofetcher/cmd/gofetcher
)
#GoTest(
#    PACKAGES github.com/m0sth8/cocaine-gofetcher/test
#)

INSTALL(
    PROGRAMS ${CMAKE_BINARY_DIR}/${PROJECT_NAME}
    DESTINATION bin COMPONENT runtime
)

CONFIGURE_FILE(${CMAKE_CURRENT_SOURCE_DIR}/app/gofetcher/cocaine.cmake.in ${CMAKE_CURRENT_BINARY_DIR}/cocaine.cmake)
IF(NOT CMAKE_BUILD_TYPE MATCHES "ReleaseWithDebInfo")
    INSTALL(SCRIPT ${CMAKE_CURRENT_BINARY_DIR}/cocaine.cmake)
ENDIF()
