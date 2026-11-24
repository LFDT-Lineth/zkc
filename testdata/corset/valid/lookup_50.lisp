;; As lookup_21, but without target selectors.
;; fragmented lookup with two two column targets
(deflookup test ((s.one s.X1 s.X2)) ((_ t1.Y1 t1.Y2) (_ t2.Z1 t2.Z2)))

(module s)
(defcolumns (X1 :i16) (X2 :i16) (one :i1 :constant 1))

(module t1)
(defcolumns (Y1 :i16) (Y2 :i16))

(module t2)
(defcolumns (Z1 :i16) (Z2 :i16))
