if(NOT DEFINED OPENSSL_EXECUTABLE OR NOT DEFINED OUTPUT_DIRECTORY)
    message(FATAL_ERROR "OPENSSL_EXECUTABLE and OUTPUT_DIRECTORY are required")
endif()

file(MAKE_DIRECTORY "${OUTPUT_DIRECTORY}")
set(config "${OUTPUT_DIRECTORY}/openssl-fixture.cnf")
file(WRITE "${config}" [=[
[req]
distinguished_name = subject
x509_extensions = extensions
prompt = no

[subject]
CN = Headroom Test Session

[extensions]
subjectAltName = @alternate_names
basicConstraints = critical,CA:TRUE
keyUsage = critical,digitalSignature,keyEncipherment,keyCertSign
extendedKeyUsage = serverAuth

[alternate_names]
DNS.1 = localhost
IP.1 = 127.0.0.1
IP.2 = ::1
]=])

function(generate_identity prefix common_name)
    set(identity_config "${OUTPUT_DIRECTORY}/${prefix}-openssl.cnf")
    file(READ "${config}" config_contents)
    string(REPLACE "CN = Headroom Test Session" "CN = ${common_name}" config_contents "${config_contents}")
    file(WRITE "${identity_config}" "${config_contents}")
    execute_process(COMMAND "${OPENSSL_EXECUTABLE}" req -x509 -newkey rsa:2048 -nodes
        -sha256 -days 365 -config "${identity_config}"
        -keyout "${OUTPUT_DIRECTORY}/${prefix}-private-key.pem"
        -out "${OUTPUT_DIRECTORY}/${prefix}-certificate.pem"
        RESULT_VARIABLE result ERROR_VARIABLE error OUTPUT_QUIET)
    if(NOT result EQUAL 0)
        message(FATAL_ERROR "Could not generate ${prefix} TLS fixture identity: ${error}")
    endif()
endfunction()

generate_identity("primary" "Headroom Test Session")
generate_identity("replacement" "Headroom Replacement Test Session")

foreach(name IN ITEMS primary-certificate primary-private-key replacement-certificate replacement-private-key)
    file(READ "${OUTPUT_DIRECTORY}/${name}.pem" "${name}")
endforeach()
file(WRITE "${OUTPUT_DIRECTORY}/headroom_macos_tls_fixture.h"
    "// Generated synthetic test identities. Do not commit generated contents.\n"
    "inline constexpr char certificatePem[] = R\"PEM(${primary-certificate})PEM\";\n"
    "inline constexpr char privateKeyPem[] = R\"PEM(${primary-private-key})PEM\";\n"
    "inline constexpr char replacementCertificatePem[] = R\"PEM(${replacement-certificate})PEM\";\n"
    "inline constexpr char replacementPrivateKeyPem[] = R\"PEM(${replacement-private-key})PEM\";\n")
