"use client";

import { useMutation, useQuery } from "@connectrpc/connect-query";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Field,
  fault,
  Input,
} from "@franco_roura/module-01";
import { type CategoryRule, ImportService } from "@ledger-01/api";
import { useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { CategoryPicker } from "@/components/category-picker";
import type { useCategories } from "@/lib/api/hooks";

/**
 * The rule for the same text, as the server compares them (spaces collapsed, any case): saving a
 * new rule for it updates that one.
 */
export function sameRule(rules: CategoryRule[], pattern: string): CategoryRule | undefined {
  const key = pattern.trim().split(/\s+/).join(" ").toLowerCase();
  return key ? rules.find((r) => r.pattern.toLowerCase() === key) : undefined;
}

/** Earlier transactions a rule could rename, offered right after it's saved. */
export type RenameOffer = { rule: CategoryRule; count: number; previousRename: string };

/**
 * Offers to rename the transactions imported before a rule got (or changed) its rename. Notes
 * edited by hand are never touched; the server decides which ones qualify.
 */
export function RenameEarlierDialog({
  offer,
  onClose,
}: {
  offer: RenameOffer | null;
  onClose: () => void;
}) {
  const rename = useMutation(ImportService.method.renameRuleTransactions);
  async function apply() {
    if (!offer) return;
    try {
      const res = await rename.mutateAsync({
        ruleId: offer.rule.id,
        previousRename: offer.previousRename,
      });
      const n = Number(res.renamedCount);
      toast.success(`Renamed ${n} ${n === 1 ? "transaction" : "transactions"}`);
    } catch {
      // toasted
    }
    onClose();
  }
  const n = offer?.count ?? 0;
  return (
    <AlertDialog open={offer !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Rename {n} earlier {n === 1 ? "transaction" : "transactions"}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            {n === 1 ? "One transaction" : `${n} transactions`} imported before still{" "}
            {n === 1 ? "has" : "have"} the bank's text. Rename {n === 1 ? "it" : "them"} to “
            {offer?.rule.rename}”? Notes you edited by hand stay as they are.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Only new imports</AlertDialogCancel>
          <AlertDialogAction disabled={rename.isPending} onClick={apply}>
            Rename {n === 1 ? "it" : "them"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * Makes a rule ("when the note contains … use … and rename to …"), prefilled from an imported
 * row, or edits one. Then offers to rename earlier transactions when the rename can apply to any.
 */
export function RuleDialog({
  rule,
  initialPattern = "",
  initialCategory = "",
  categories,
  walletId,
  onClose,
  onSaved,
}: {
  /** The rule to edit; without it the dialog makes a new one. */
  rule?: CategoryRule;
  initialPattern?: string;
  initialCategory?: string;
  categories: ReturnType<typeof useCategories>["categories"];
  walletId: string;
  onClose: () => void;
  onSaved?: (rule: CategoryRule) => void;
}) {
  const id = useId();
  const [pattern, setPattern] = useState(rule?.pattern ?? initialPattern);
  const [categoryId, setCategoryId] = useState(rule?.categoryId ?? initialCategory);
  const [renameTo, setRenameTo] = useState(rule?.rename ?? "");
  const [renameTouched, setRenameTouched] = useState(Boolean(rule));
  const [errors, setErrors] = useState<{ pattern?: string; cat?: string }>({});
  const [offer, setOffer] = useState<RenameOffer | null>(null);
  const create = useMutation(ImportService.method.createCategoryRule);
  const update = useMutation(ImportService.method.updateCategoryRule);
  // A new rule for text that has one already updates that rule (the server upserts), so show it.
  const { data } = useQuery(ImportService.method.listCategoryRules, {}, { enabled: !rule });
  const existing = rule ? undefined : sameRule(data?.rules ?? [], pattern);
  useEffect(() => {
    if (!existing) return;
    if (!renameTouched) setRenameTo(existing.rename);
    setCategoryId((c) => c || existing.categoryId);
  }, [existing, renameTouched]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!pattern.trim()) {
      setErrors({ pattern: "Enter the text to look for" });
      return fault(`${id}-pattern`);
    }
    if (!categoryId) {
      setErrors({ cat: "Choose a category" });
      return fault(`${id}-cat`);
    }
    let saved: CategoryRule | undefined;
    let count = 0;
    let previousRename = "";
    try {
      const res = rule
        ? await update.mutateAsync({ id: rule.id, pattern, categoryId, rename: renameTo })
        : await create.mutateAsync({ pattern, categoryId, rename: renameTo });
      saved = res.rule;
      count = res.renamable;
      previousRename = "previousRename" in res ? res.previousRename : (rule?.rename ?? "");
    } catch {
      return; // toasted
    }
    if (!saved) return;
    toast.success(rule || existing ? "Rule updated" : "Rule saved");
    onSaved?.(saved);
    if (count > 0) setOffer({ rule: saved, count, previousRename });
    else onClose();
  }

  return (
    <>
      <Dialog open={offer === null} onOpenChange={(open) => !open && onClose()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{rule || existing ? "Edit rule" : "Make a rule"}</DialogTitle>
            <DialogDescription>
              Rows whose note contains this text get this category, and the name if you give one,
              now and on later imports.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={save} className="flex flex-col gap-4">
            <Field
              label="When the note contains"
              htmlFor={`${id}-pattern`}
              error={errors.pattern}
              help={
                existing ? "There's a rule for this text already; saving updates it." : undefined
              }
            >
              <Input
                id={`${id}-pattern`}
                value={pattern}
                maxLength={100}
                onChange={(e) => {
                  setPattern(e.target.value);
                  setErrors({});
                }}
              />
            </Field>
            <Field label="Use the category" htmlFor={`${id}-cat`} error={errors.cat}>
              <CategoryPicker
                id={`${id}-cat`}
                categories={categories}
                walletId={walletId}
                value={categoryId}
                onChange={(v) => {
                  setCategoryId(v);
                  setErrors({});
                }}
              />
            </Field>
            <Field label="Rename to (optional)" htmlFor={`${id}-rename`}>
              <Input
                id={`${id}-rename`}
                value={renameTo}
                maxLength={100}
                placeholder="Keep the bank's text"
                onChange={(e) => {
                  setRenameTo(e.target.value);
                  setRenameTouched(true);
                }}
              />
            </Field>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={create.isPending || update.isPending}>
                Save rule
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <RenameEarlierDialog offer={offer} onClose={onClose} />
    </>
  );
}
