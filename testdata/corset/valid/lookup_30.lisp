(module m1)
(defcolumns (sel :i1) (X :i16) (Y :i8) (tmp :i8))
;; tmp holds the (narrowed) value of X on selected rows
(defconstraint tmp_def () (if (== sel 1) (== tmp X)))

(deflookup
  l1
  ;; source vector (selector, then columns)
  ((m1.sel m1.tmp m1.Y))
  ;; target vector (selector, then columns)
  ((m2.one m2.X m2.Y)))

(module m2)
(defcolumns (X :i8) (Y :i8) (one :i1 :constant 1))
