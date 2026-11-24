(defcolumns (sel :i1) (from :i16) (into :i8) (tmp :i8) (one :i1 :constant 1))
;; tmp holds the (narrowed) value of from on selected rows
(defconstraint tmp_def () (if (== sel 1) (== tmp from)))

(deflookup
  l1
  ;; source vector (selector, then column)
  ((sel tmp))
  ;; target vector (selector, then column)
  ((one into))
)
