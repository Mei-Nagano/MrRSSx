package com.wails.app;

import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;

/**
 * Decodes the optional metadata envelope returned for native webpage resources.
 * The envelope keeps the existing byte-only JNI method while preserving the
 * upstream Content-Type for resources whose URLs have no useful extension.
 */
final class ResourceMetadata {
    final String mimeType;
    final byte[] body;

    private ResourceMetadata(String mimeType, byte[] body) {
        this.mimeType = mimeType;
        this.body = body;
    }

    static ResourceMetadata decode(byte[] payload) {
        if (payload == null || payload.length == 0) {
            return null;
        }
        String json = new String(payload, StandardCharsets.UTF_8);
        String encodedBody = stringField(json, "body");
        if (encodedBody == null) {
            return null;
        }
        byte[] body = decodeBase64(encodedBody);
        if (body == null) {
            return null;
        }
        return new ResourceMetadata(
                ResourceMime.fromContentType(stringField(json, "content_type")), body);
    }

    private static String stringField(String json, String name) {
        String key = "\"" + name + "\"";
        int index = json.indexOf(key);
        if (index < 0) {
            return null;
        }
        index += key.length();
        while (index < json.length() && Character.isWhitespace(json.charAt(index))) index++;
        if (index >= json.length() || json.charAt(index++) != ':') return null;
        while (index < json.length() && Character.isWhitespace(json.charAt(index))) index++;
        if (index >= json.length() || json.charAt(index++) != '"') return null;

        StringBuilder value = new StringBuilder();
        while (index < json.length()) {
            char current = json.charAt(index++);
            if (current == '"') return value.toString();
            if (current != '\\') {
                if (current < 0x20) return null;
                value.append(current);
                continue;
            }
            if (index >= json.length()) return null;
            char escaped = json.charAt(index++);
            switch (escaped) {
                case '"': case '\\': case '/': value.append(escaped); break;
                case 'b': value.append('\b'); break;
                case 'f': value.append('\f'); break;
                case 'n': value.append('\n'); break;
                case 'r': value.append('\r'); break;
                case 't': value.append('\t'); break;
                case 'u':
                    if (index + 4 > json.length()) return null;
                    try {
                        value.append((char) Integer.parseInt(json.substring(index, index + 4), 16));
                    } catch (NumberFormatException e) {
                        return null;
                    }
                    index += 4;
                    break;
                default: return null;
            }
        }
        return null;
    }

    private static byte[] decodeBase64(String value) {
        if (value.length() % 4 != 0) return null;
        ByteArrayOutputStream output = new ByteArrayOutputStream(value.length() * 3 / 4);
        for (int index = 0; index < value.length(); index += 4) {
            int first = base64Value(value.charAt(index));
            int second = base64Value(value.charAt(index + 1));
            int third = value.charAt(index + 2) == '=' ? -2 : base64Value(value.charAt(index + 2));
            int fourth = value.charAt(index + 3) == '=' ? -2 : base64Value(value.charAt(index + 3));
            if (first < 0 || second < 0 || third == -1 || fourth == -1
                    || (third == -2 && fourth != -2)) return null;
            output.write((first << 2) | (second >> 4));
            if (third >= 0) output.write((second << 4) | (third >> 2));
            if (fourth >= 0) output.write((third << 6) | fourth);
        }
        return output.toByteArray();
    }

    private static int base64Value(char value) {
        if (value >= 'A' && value <= 'Z') return value - 'A';
        if (value >= 'a' && value <= 'z') return value - 'a' + 26;
        if (value >= '0' && value <= '9') return value - '0' + 52;
        if (value == '+') return 62;
        if (value == '/') return 63;
        return -1;
    }
}
