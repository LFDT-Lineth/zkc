;; As lookup_18, but with only one target having a selector.
(defcolumns (X :i16) (Y :i16) (Z :i16) (one :i1 :constant 1))
;; fragmented lookup with two targets
(deflookup test ((_ X)) ((_ Y) (one Z)))
