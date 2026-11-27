;; As permutation_29, but without a target selector.
(defcolumns (sel :i1) (from :i16) (into :i8) (tmp :i8))
;; tmp holds the (narrowed) value of from on selected rows.
(defconstraint tmp_def () (if (== sel 1) (== tmp from)))

(defpermutation
  l1
  ;; Source vector (selector, then column).
  ((sel tmp))
  ;; Target vector (no selector, then column).
  ((_ into))
)
