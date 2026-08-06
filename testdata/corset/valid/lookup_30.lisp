(module m1)
(defcolumns (sel :i1) (X :i16) (Y :i8) (tmp :i8))
;; tmp holds the (narrowed) value of X on selected rows
(defconstraint tmp_def () (if (== sel 1) (== tmp X)))

(defclookup
  l1
  ;; target column
  (m2.X m2.Y)
  ;; source selector
  m1.sel
  ;; source column
  (m1.tmp m1.Y))

(module m2)
(defcolumns (X :i8) (Y :i8))
