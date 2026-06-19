import { type FormEvent, useState } from "react";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { FieldGroup } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { predict } from "@/lib/api";
import type { ComponentSchema, InterfaceSchema } from "@/types";
import { initialOutputs, outputShowcaseInputs, type DemoMode } from "@/features/demo/demo-mode";
import { LayoutBlock } from "@/features/schema/LayoutBlock";
import { OutputBlock } from "@/features/schema/OutputBlock";
import { SchemaInput } from "@/features/schema/SchemaInput";
import {
  errorMessage,
  flattenLeafComponents,
  isLayoutComponent,
  type Outputs,
  type Values,
  useInitialValues,
} from "@/features/schema/schema-utils";

export function FormInterface({ iface, demoMode }: { iface: InterfaceSchema; demoMode: DemoMode }) {
  const isOutputShowcase = demoMode === "readme-outputs";
  const renderedInputs = isOutputShowcase ? outputShowcaseInputs(iface.inputs) : iface.inputs;
  const inputFields = flattenLeafComponents(renderedInputs);
  const outputFields = flattenLeafComponents(iface.outputs);
  const [values, setValues] = useInitialValues(inputFields);
  const [outputs, setOutputs] = useState<Outputs>(() => initialOutputs(outputFields, demoMode));
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const hasOutputValues = outputFields.some((component) => outputs[component.id] !== undefined);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await predict(
        iface.id,
        inputFields.map((component) => values[component.id] ?? ""),
      );
      setOutputs(Object.fromEntries(outputFields.map((component, index) => [component.id, result[index]])));
    } catch (submitError) {
      setError(errorMessage(submitError));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <Card className="overflow-hidden border-border/80 shadow-sm">
      <CardHeader className="border-b bg-card/70">
        <CardTitle>Interface</CardTitle>
        <CardDescription>{iface.id}</CardDescription>
      </CardHeader>
      <form onSubmit={onSubmit}>
        <CardContent
          className={
            hasOutputValues
              ? isOutputShowcase
                ? "grid gap-6 pt-6 lg:grid-cols-[0.78fr,1.22fr]"
                : "grid gap-6 pt-6 lg:grid-cols-[1.08fr,0.92fr]"
              : "pt-6"
          }
        >
          <FieldGroup className="gap-5">
            {renderSchemaInputs(renderedInputs, isSubmitting, values, (component, value) => {
              setValues((current) => ({ ...current, [component.id]: value }));
            })}
            {error ? <ErrorAlert title="Request failed" message={error} /> : null}
          </FieldGroup>
          {hasOutputValues ? (
            <section className="flex flex-col gap-3">
              {renderSchemaOutputs(iface.outputs, outputs).map((node) => (
                <OutputBlock key={node.key} component={node.component} value={node.value} />
              ))}
            </section>
          ) : null}
        </CardContent>
        <CardFooter className="border-t bg-card/55">
          <Button type="submit" disabled={isSubmitting}>
            {isSubmitting ? <Spinner /> : null}
            {isSubmitting ? "Running" : "Run"}
          </Button>
        </CardFooter>
      </form>
    </Card>
  );
}

function renderSchemaInputs(
  components: ComponentSchema[],
  disabled: boolean,
  values: Values,
  onChange: (component: ComponentSchema, value: unknown) => void,
) {
  return components.map((component) => {
    if (isLayoutComponent(component.type)) {
      return (
        <LayoutBlock key={component.id} type={component.type} label={component.label}>
          {renderSchemaInputs(component.items ?? [], disabled, values, onChange)}
        </LayoutBlock>
      );
    }

    return (
      <SchemaInput
        key={component.id}
        component={component}
        disabled={disabled}
        value={values[component.id]}
        onChange={(value) => onChange(component, value)}
      />
    );
  });
}

function renderSchemaOutputs(
  components: ComponentSchema[],
  outputs: Outputs,
): { key: string; component: ComponentSchema; value: unknown }[] {
  const rows: { key: string; component: ComponentSchema; value: unknown }[] = [];

  for (const component of components) {
    if (isLayoutComponent(component.type)) {
      if (component.items?.length) {
        rows.push(...renderSchemaOutputs(component.items, outputs));
      }
      continue;
    }

    rows.push({
      key: component.id,
      component,
      value: outputs[component.id],
    });
  }

  return rows;
}
