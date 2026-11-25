;; As permutation_30, but without a target selector.
(module m1)
(defcolumns (sel :i1) (X :i16) (Y :i8) (tmp :i8))
;; tmp holds the (narrowed) value of X on selected rows.
(defconstraint tmp_def () (if (== sel 1) (== tmp X)))

(defpermutation
  l1
  ;; Source vector (selector, then columns).
  ((m1.sel m1.tmp m1.Y))
  ;; Target vector (no selector, then columns).
  ((_ m2.X m2.Y)))

(module m2)
(defcolumns (X :i8) (Y :i8))
