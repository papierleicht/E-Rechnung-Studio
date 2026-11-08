import java.nio.file.*;
import java.util.*;
import javax.xml.transform.*;
import javax.xml.transform.stream.*;

// Test-only runner. Runtime validation in E-Rechnung-Studio has no Java dependency.
class ReferenceSchematron {
    public static void main(String[] args) throws Exception {
        TransformerFactory factory = new net.sf.saxon.TransformerFactoryImpl();
        Map<String, Templates> compiled = new HashMap<>();
        for (String line : Files.readAllLines(Path.of(args[0]))) {
            String[] row = line.split("\t");
            Templates template = compiled.get(row[1]);
            if (template == null) {
                template = factory.newTemplates(new StreamSource(row[1]));
                compiled.put(row[1], template);
            }
            template.newTransformer().transform(new StreamSource(row[0]), new StreamResult(row[2]));
        }
    }
}
